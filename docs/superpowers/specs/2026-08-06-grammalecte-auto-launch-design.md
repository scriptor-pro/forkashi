# Auto-lancement du serveur Grammalecte

**Date** : 2026-08-06
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

## Contexte

`grammalecteBackend` (`grammar_grammalecte.go`) est un client HTTP pur : il se connecte à un serveur
Grammalecte local (`localhost:8080` par défaut) mais ne le lance jamais lui-même. Le commentaire en
tête de fichier le documente explicitement : *« started and stopped manually by the user (v1) »*. La
spec forkashi v1 (`docs/superpowers/specs/2026-08-01-forkashi-v1-design.md` §3, §6) notait déjà ce
manque comme amélioration future hors-périmètre.

En pratique, l'utilisateur n'a jamais lancé Grammalecte manuellement faute de savoir comment — le
backend est resté silencieusement indisponible depuis son introduction. Une session de recherche
(2026-08-06) a confirmé sur la machine de l'utilisateur :

- Le script serveur officiel vit à `/home/Baudouin/Apps/grammalecte-server.py` (paquet téléchargé
  depuis grammalecte.net, pas de distribution pip, pas de dépôt GitHub canonique).
- Commande de lancement confirmée par test réel : `python3 /home/Baudouin/Apps/grammalecte-server.py`
  (exécuté depuis `/home/Baudouin/Apps/`, le script important `grammalecte` comme paquet relatif).
- Port par défaut 8080, endpoint `POST /gc_text/fr` — exactement ce que `grammar_grammalecte.go`
  attend déjà, aucun changement de protocole nécessaire.
- Démarrage lent : ~5-6 secondes avant que le port réponde (chargement du moteur linguistique
  Grammalecte). Le script imprime des informations sur stdout pendant ce temps mais n'a pas de
  marqueur "prêt" fiable autre que le port qui répond.
- Aucune dépendance pip externe : `bottle.py` (le micro-framework HTTP) est vendorisé dans le paquet
  Grammalecte lui-même.

Ce document définit comment okashi peut lancer et gérer ce serveur lui-même, sans intervention
manuelle de l'utilisateur.

## 1. Configuration : `OKASHI_GRAMMALECTE_CMD`

Nouvelle variable d'environnement, dans le style des `OKASHI_GRAMMALECTE_HOST`/`OKASHI_GRAMMALECTE_PORT`
déjà existants (`grammar_grammalecte.go:22-34`) : la commande complète à exécuter pour démarrer le
serveur, par exemple :

```
OKASHI_GRAMMALECTE_CMD="python3 /home/Baudouin/Apps/grammalecte-server.py"
```

- **Non définie** : comportement actuel préservé à l'identique — okashi sonde `localhost:8080` (ou
  l'hôte/port configurés) une fois au démarrage, aucun lancement automatique. Un message d'aide
  discret indique que l'auto-lancement peut être activé via cette variable (affiché là où l'état du
  backend grammaire est déjà visible aujourd'hui — l'inspector — pas une popup intrusive).
- **Définie** : gouverne le comportement décrit ci-dessous.
- Parsing simple : la commande est découpée en programme + arguments (`strings.Fields` ou équivalent),
  **pas** un shell complet — aucun support de `&&`, `|`, redirections, ou substitution de variables
  internes à la chaîne. Une commande simple avec arguments positionnels suffit au cas d'usage.

## 2. Séquence au démarrage

1. `initialModel()` sonde `Available()` sur le serveur configuré (`localhost:8080` par défaut), **exactement
   comme aujourd'hui** — cette étape ne change pas.
2. **Si déjà disponible** : okashi l'utilise directement, qu'il ait été lancé manuellement ou par une
   session okashi précédente. Ce serveur n'est **jamais** considéré comme la propriété d'okashi : il
   ne sera pas tué à la fermeture (voir §4). Aucun nouveau sous-processus n'est lancé.
3. **Si indisponible ET `OKASHI_GRAMMALECTE_CMD` définie** : okashi lance le sous-processus
   (`exec.Command(...).Start()`, non-bloquant, stdin/stdout/stderr non capturés pour ne pas perturber
   le rendu du TUI) et conserve la référence `*exec.Cmd` dans le modèle — c'est ce qui distingue "mon
   enfant, à tuer en sortant" de "serveur externe, à laisser vivre".
4. **Si indisponible ET pas de commande configurée** : identique à aujourd'hui (`m.grammarChecker = nil`),
   plus le message d'aide discret du §1.

Dans tous les cas, `initialModel()` retourne **immédiatement** (non-bloquant) — l'écran d'accueil
s'affiche sans attendre les ~5-6 secondes de démarrage du serveur. `m.grammarChecker` reste `nil`
jusqu'à confirmation de disponibilité (§3).

## 3. Détection de disponibilité (polling non-bloquant)

Suit le pattern `tea.Cmd`/`tea.Tick` déjà utilisé ailleurs dans le fichier (ex. `checkGrammarCmd`,
`main.go:684`) — pas de goroutine ni de channel manuel, cohérent avec l'architecture Elm du reste du
code :

```go
type grammalecteReadyMsg struct{}

// pollGrammalecteCmd checks whether the auto-launched Grammalecte server has become reachable
// yet, and re-schedules itself every 500ms until it is or the attempts run out. attempt starts
// at 0; polling stops silently once it exceeds the ceiling (grammalectePollMaxAttempts) — the
// launched process may have crashed, and best-effort means okashi keeps running without French
// grammar checking rather than polling forever.
const grammalectePollMaxAttempts = 60 // 60 × 500ms = 30s

func pollGrammalecteCmd(gc grammarChecker, attempt int) tea.Cmd {
	return func() tea.Msg {
		if gc.Available() {
			return grammalecteReadyMsg{}
		}
		if attempt >= grammalectePollMaxAttempts {
			return nil // give up silently — best-effort, same philosophy as Available() itself
		}
		time.Sleep(500 * time.Millisecond)
		return pollGrammalecteCmd(gc, attempt+1)()
	}
}
```

(Forme exacte à raffiner en plan d'implémentation — l'important est le contrat : re-sonde toutes les
500ms, jusqu'à 60 tentatives soit 30 secondes, silencieux à l'échec comme au succès tardif.)

Dans `Update()`, sur réception de `grammalecteReadyMsg{}` : assigner `m.grammarChecker = gc` (le
checker est déjà construit — seule sa disponibilité était en attente) et rafraîchir
`m.inspector.grammarBackend`. Ce polling ne démarre **que** dans le cas §2.3 (sous-processus lancé par
okashi) — pas dans le cas où le serveur était déjà disponible dès le départ (rien à attendre), ni dans
le cas sans commande configurée (rien à lancer).

## 4. Nettoyage à la fermeture

Le champ `*exec.Cmd` (non-nil uniquement si okashi a lancé le sous-processus lui-même, §2.3) est le
marqueur de propriété. Dans `func main()`, après le retour de `p.Run()` : si ce champ est non-nil,
envoyer `SIGTERM` au process, avec un `Kill()` de repli après un court délai s'il ne se termine pas
de lui-même. Un serveur déjà présent au démarrage (§2.2) n'est jamais touché ici — il n'appartient pas
à okashi, peu importe qui l'a lancé.

## 5. Échecs — philosophie best-effort

Cohérent avec `Available()` existant (`grammar_grammalecte.go:49-56`, déjà commenté *"okashi must run
fine without French grammar checking rather than crash"*) : aucune erreur visible à l'utilisateur en
cas de commande introuvable, script absent, erreur Python au démarrage, ou serveur qui ne répond
jamais dans le délai du polling. `m.grammarChecker` reste `nil`, l'inspector continue d'afficher l'état
"indisponible" comme il le fait déjà pour n'importe quelle indisponibilité de Grammalecte.

## Hors périmètre

- Pas de configuration par projet (Properties) — uniquement variable d'environnement globale, cohérent
  avec `OKASHI_GRAMMALECTE_HOST`/`OKASHI_GRAMMALECTE_PORT` existants.
- Pas de redémarrage automatique si le serveur meurt en cours de session après avoir démarré avec
  succès — seul le lancement initial est couvert.
- Pas de parsing shell complet pour `OKASHI_GRAMMALECTE_CMD` (pas de `&&`, `|`, redirections,
  variables shell internes) — commande simple + arguments positionnels uniquement.
- Pas de message d'erreur visible à l'utilisateur en cas d'échec (§5) — écarté explicitement en faveur
  du silence best-effort, cohérent avec le comportement `Available()` déjà en place.

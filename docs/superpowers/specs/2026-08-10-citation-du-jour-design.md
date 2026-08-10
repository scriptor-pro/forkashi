# Citation du jour sur le hub

**Date** : 2026-08-10
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

## Contexte

Le hub (`home.go` / `homeContent()`) affiche le logo `bannerArt` centré, suivi d'une ligne vide,
puis les strips ÉPINGLÉS/RÉCENT et les colonnes BIBLIOTHÈQUE/FICHIERS (`home.go:1099-1142`). Une
capture d'écran annotée par l'utilisateur situe l'emplacement souhaité pour une citation sur
l'écriture : juste sous le logo, dans l'espace de la ligne vide actuelle.

Après une session de sélection collaborative (voir la conversation associée), 38 citations sur
l'écriture et la persévérance/motivation ont été retenues, vérifiées (recherche web pour les
attributions douteuses — plusieurs candidates écartées faute de source retrouvée), et ordonnées
selon une alternance thème (écriture / persévérance) et longueur (courte → moyenne → longue en
cycle), afin d'éviter deux citations similaires ou deux pavés de texte consécutifs. Ce document
définit comment cette liste ordonnée est stockée et affichée, une citation différente par jour
civil, en cycle sur 38 jours.

## 1. Stockage des citations : `quotes.json` + `quotes.go`

Un fichier `quotes.json` à la racine du module contient les 38 citations, **dans l'ordre final
déjà déterminé** (l'ordre du fichier EST l'ordre du cycle — aucun champ d'index séparé) :

```json
[
  { "text": "Écrire, c'est une façon de parler sans être interrompu.", "author": "Jules Renard" },
  { "text": "Il n'y a pas de vent favorable pour celui qui ne sait où il va.", "author": "Sénèque" }
]
```

Nouveau fichier `quotes.go` :

```go
type quote struct {
    Text   string `json:"text"`
    Author string `json:"author"`
}

//go:embed quotes.json
var quotesJSON []byte

// loadQuotes parses the embedded quote set. A parse failure (should not happen for an
// embedded, build-time-checked asset) yields an empty slice — the quote line simply
// doesn't render, mirroring the tolerant missing/corrupt → zero value pattern used by
// loadUserConfig.
func loadQuotes() []quote {
    var qs []quote
    if err := json.Unmarshal(quotesJSON, &qs); err != nil {
        return nil
    }
    return qs
}
```

Choix : JSON embarqué (plutôt qu'une slice Go littérale) pour séparer les données de contenu du
code, éditable sans recompiler mentalement une syntaxe Go — cohérent avec l'usage de `go:embed`
déjà présent dans `fonts.go` pour des assets statiques.

## 2. Calcul de l'index du jour : premier lancement + cycle de 38 jours

Nouveau champ sur `userConfig` (`settings.go`), personnel et machine-global comme `Author`/`Contact` :

```go
type userConfig struct {
    Author      string `json:"author,omitempty"`
    Contact     string `json:"contact,omitempty"`
    FirstLaunch string `json:"firstLaunch,omitempty"` // "2006-01-02"
}
```

Nouvelle fonction dans `quotes.go`, testable indépendamment de l'I/O (même séparation logique/IO
que `mergeSettings` vs `resolveSettings`) :

```go
// quoteIndexForToday resolves which of the 38 quotes to show today. today is injected for
// testability. hasConfigDir distinguishes "no persistable config" (env without a user config
// dir) from "config dir exists but firstLaunch not yet recorded".
func quoteIndexForToday(uc userConfig, today time.Time, hasConfigDir bool, n int) (index int, needsSave bool)
```

Logique :

1. **`hasConfigDir` faux** (échec de `os.UserConfigDir()`) → fallback sans état :
   `index = today.YearDay() % n`, `needsSave = false`.
2. **`uc.FirstLaunch` vide** (premier lancement, ou config illisible/corrompue — même traitement
   tolérant que le reste de `settings.go`) → `index = 0` (jour 1 du cycle), `needsSave = true`
   (appelant écrit `FirstLaunch = today` dans `config.json`).
3. **`uc.FirstLaunch` renseignée et parseable** → `days := today.Sub(firstLaunch).Hours() / 24`,
   `index = int(days) % n` (modulo protégé contre un `FirstLaunch` dans le futur ou une horloge
   qui recule : valeur absolue avant modulo).
4. **`uc.FirstLaunch` renseignée mais illisible** (corruption manuelle) → traité comme le cas 2
   (réinitialise le cycle à aujourd'hui).

`initialModel()` appelle cette fonction une fois au démarrage, résout `today.YearDay() % n` ou le
vrai cycle selon le cas, peuple `m.todayQuote = quotes[index]`, et si `needsSave`, persiste
`FirstLaunch` via `saveUserConfig` (best-effort, une erreur d'écriture ne bloque pas le démarrage —
la citation du jour s'affiche quand même, elle repartira simplement du jour 1 au lancement suivant).

## 3. Rendu dans `home.go`

Nouveau champ sur `model` : `todayQuote quote` (résolu une fois à l'`initialModel()`, jamais
recalculé en `View()` — cohérent avec le thème détecté une fois au démarrage et le principe
`View()` = O(visible)).

Dans `homeContent()`, la ligne vide qui suit le logo (`home.go:1110`) devient, quand
`m.todayQuote.Text != ""` :

```go
if m.todayQuote.Text != "" {
    q := quoteStyle.Width(blockW).Align(lipgloss.Center).Render("« " + m.todayQuote.Text + " »")
    lines = append(lines, strings.Split(q, "\n")...)
    a := quoteAuthorStyle.Width(blockW).Align(lipgloss.Center).Render("— " + m.todayQuote.Author)
    lines = append(lines, a)
}
lines = append(lines, "")
```

Nouveaux styles dans `styles.go` :

```go
var quoteStyle = lipgloss.NewStyle().
    Foreground(subtle).
    Italic(true)

var quoteAuthorStyle = lipgloss.NewStyle().
    Foreground(subtle)
```

- Guillemets français (« ») autour du texte, cohérents avec le reste de l'interface en français.
- Wrap : `lipgloss` enveloppe nativement le texte à la largeur fixée par `.Width(blockW)`. Aucune
  troncature — une citation qui dépasse 2 lignes (fenêtre étroite, ou une des citations longues
  comme Yourcenar/Duras) s'affiche sur autant de lignes que nécessaire ; le reste du hub (RECENT,
  BIBLIOTHÈQUE) se décale d'autant, comme n'importe quel contenu de hauteur variable dans ce bloc
  (le hub n'est déjà pas de hauteur fixe — `colH` dépend du contenu des colonnes).
- Auteur sur une ligne séparée sous le texte, non-italique, pour une distinction visuelle nette
  entre citation et attribution.

## Annexe : contenu de `quotes.json` (ordre final, 38 entrées)

Ordre déterminé par alternance thème (écriture / persévérance) et longueur (courte → moyenne →
longue en cycle). Attributions marquées *(source précise non retrouvée)* pour les citations
largement attestées dans les recueils mais sans référence primaire vérifiée lors de la recherche
du 2026-08-10 ; conservées à dessein car l'usage est solidement établi, contrairement aux
candidates écartées durant la sélection (Beauvoir, Bobin, Colette, Maupassant — non retrouvées,
non retenues).

1. « Écrire, c'est une façon de parler sans être interrompu. » — Jules Renard
2. « L'inspiration existe, mais il faut qu'elle vous trouve en train de travailler. » — Pablo Picasso *(source précise non retrouvée)*
3. « Il n'y a pas de grande œuvre qui ne soit le fruit d'une obstination. » — Colette
4. « Un écrivain, c'est quelqu'un pour qui écrire est plus difficile que pour les autres. » — Thomas Mann *(source précise non retrouvée)*
5. « J'écris pour me délivrer, pour ordonner un chaos qui, autrement, resterait obscur. » — Marguerite Yourcenar
6. « La page blanche n'existe pas ; il n'y a que des débuts qu'on n'a pas encore osé écrire. » — inspirée d'Anne Hébert *(source précise non retrouvée)*
7. « Écris ce que tu ne dois pas oublier. » — Isabel Allende
8. « Vous pouvez toujours corriger une mauvaise page. Vous ne pouvez rien tirer d'une page blanche. » — Jodi Picoult
9. « The first draft of anything is shit. » — Ernest Hemingway
10. « N'attends pas d'être inspiré. Assieds-toi et mets-toi au travail. » — Stephen King
11. « Un écrivain n'est jamais aussi bon que ses meilleures pages, ni aussi mauvais que ses pires. » — Ernest Hemingway
12. « Le talent, c'est 1% d'inspiration et 99% de transpiration. » — Thomas Edison
13. « Ce n'est pas parce que les choses sont difficiles que nous n'osons pas, c'est parce que nous n'osons pas qu'elles sont difficiles. » — Sénèque
14. « Il n'y a pas de vent favorable pour celui qui ne sait où il va. » — Sénèque
15. « Tomber sept fois, se relever huit. » — proverbe
16. « La persévérance est un talent tout comme les autres, et peut-être plus rare. » — Louis Pergaud *(source précise non retrouvée)*
17. « On ne subit pas l'avenir, on le fait. » — Georges Bernanos
18. « Il faut beaucoup de patience et un peu d'audace pour aller jusqu'au bout de ce qu'on a commencé. » — source incertaine, non attribuée
19. « Notre plus grande gloire n'est pas de ne jamais tomber, mais de nous relever à chaque chute. » — Confucius *(traduite)*
20. « Le succès, c'est se déplacer d'échec en échec sans perdre son enthousiasme. » — Winston Churchill
21. « Ce n'est pas la charge qui vous casse, c'est la façon dont vous la portez. » — Lou Holtz
22. « La différence entre l'ordinaire et l'extraordinaire, c'est ce petit extra. » — Jimmy Johnson
23. « Il n'est jamais trop tard pour être ce que tu aurais pu être. » — George Eliot
24. « Beaucoup d'échecs dans la vie sont dus à des gens qui ne réalisaient pas à quel point ils étaient proches du succès quand ils ont abandonné. » — Thomas Edison
25. « Continue d'avancer. Ne t'arrête jamais. » — Walt Disney
26. « Écrire, c'est une façon de vivre deux fois. » — Anaïs Nin
27. « Il n'y a pas d'ascenseur pour la réussite, il faut prendre l'escalier. » — Zig Ziglar
28. « La qualité n'est jamais un accident ; elle est toujours le résultat d'un effort intelligent. » — John Ruskin
29. « Le voyage de mille lieues commence toujours par un premier pas. » — Lao Tseu *(traduite)*
30. « Fais de ton mieux jusqu'à ce que tu saches faire mieux. Alors, quand tu sais mieux, fais mieux. » — Maya Angelou
31. « Le succès n'est pas final, l'échec n'est pas fatal : c'est le courage de continuer qui compte. » — Winston Churchill
32. « Il faut toujours viser la lune, car même en cas d'échec, on atterrit dans les étoiles. » — Oscar Wilde *(traduite)*
33. « La motivation, c'est ce qui vous permet de commencer. L'habitude, c'est ce qui vous permet de continuer. » — Jim Ryun *(traduite)*
34. « Un roman, c'est un miroir qu'on promène le long d'un chemin. » — Stendhal *(épigraphe apocryphe du Rouge et le Noir, attribuée par Stendhal lui-même à « Saint-Réal », usage courant l'attribue à Stendhal)*
35. « Écrire, c'est tenter de savoir ce qu'on écrirait si on écrivait — on ne le sait qu'après. » — Marguerite Duras *(Écrire, Gallimard, 1993)*
36. « Un livre doit être la hache pour la mer gelée en nous. » — Franz Kafka *(traduite ; lettre à Oskar Pollak, janvier 1904)*
37. « Un livre est un miroir. Si un singe s'y regarde, ce n'est pas l'image d'un apôtre qui apparaît. » — Georg Christoph Lichtenberg *(traduite)*
38. « L'écriture est la peinture de la voix. » — Voltaire *(source précise non retrouvée)*

## Hors périmètre

- Pas de rotation manuelle (touche pour "citation suivante") — v1 = une par jour, pas d'interaction.
- Pas de configuration pour désactiver l'affichage (`OKASHI_*` ou Properties) — periemtre géré si
  demandé après usage réel.
- Le cycle de 38 jours n'est pas aligné sur un calendrier mensuel réel malgré l'intention initiale
  "une par jour pendant un mois" — 38 > 31, le cycle déborde sur le mois suivant avant de
  recommencer. Accepté tel quel (pas de troncature à 31 ni de sélection différente par mois).

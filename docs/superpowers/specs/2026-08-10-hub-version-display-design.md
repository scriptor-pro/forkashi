# Numéro de version sur le hub

**Date** : 2026-08-10
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

## Contexte

`main.go:30` déclare déjà `var version = "dev"`, overridée au build via
`-ldflags "-X main.version=…"` (commentaire `main.go:28-29` : la formule Homebrew injecte le tag
de release ici). `--version`/`-v` (`main.go:3134-3135`) affiche déjà `"okashi " + version`. Ce
mécanisme d'injection existe et fonctionne — rien à construire de ce côté.

Une capture d'écran annotée par l'utilisateur demande d'afficher ce même numéro sur l'écran
d'accueil (hub), sur la même ligne que le mot-symbole « o k a s h i », légèrement décalé à droite
(pas collé). Le logo actuel (`styles.go:57-58`, `bannerArt`) est un bloc de 2 lignes :

```
o k a s h i
───────────
```

rendu ligne par ligne et centré sur `blockW` dans `homeContent()` (`home.go:1107-1109`), juste
avant le rendu de la citation du jour ajoutée précédemment (`home.go:1110-1117`). La capture montre
la version uniquement sur la 1re ligne — la règle du dessous reste courte, elle ne s'étend pas sous
le numéro de version.

## Rendu dans `homeContent()`

`home.go:1107-1109` change de :

```go
for _, l := range strings.Split(bannerArt, "\n") {
    lines = append(lines, pad(bannerStyle.Render(l)))
}
```

à un traitement qui distingue la 1re ligne (logo + version) des lignes suivantes (règle, inchangée) :

```go
bannerLines := strings.Split(bannerArt, "\n")
versionLabel := version
if version != "dev" {
    versionLabel = "v" + version
}
first := bannerStyle.Render(bannerLines[0]) + "  " + versionStyle.Render(versionLabel)
lines = append(lines, pad(first))
for _, l := range bannerLines[1:] {
    lines = append(lines, pad(bannerStyle.Render(l)))
}
```

- `version` est la variable package-level existante (`main.go:30`) — accessible directement depuis
  `homeContent()` (méthode de `model`, même package `main`), aucun nouveau champ sur `model`.
- Deux espaces littéraux séparent le logo et le numéro de version (espacement fixe, pas de
  positionnement au bord droit du bloc — cohérent avec le rendu déjà centré du logo entier).
- Le bloc concaténé (`"o k a s h i" + "  " + "v0.1.126"`, ou `"o k a s h i" + "  " + "dev"`) est
  centré comme un tout via `pad()`, exactement le même mécanisme que le logo seul aujourd'hui — le
  bloc s'élargit, mais le calcul de centrage ne change pas.
- La 2e ligne (`───────────`) est rendue sans modification, donc reste plus courte que la 1re ligne
  augmentée — correspond à la capture.

## Cas `version == "dev"`

Un binaire compilé sans `-ldflags` (tous les builds locaux actuels de l'utilisateur) affiche
`"o k a s h i  dev"` — pas de préfixe `v` devant `dev` (le préfixe `v` n'a de sens que devant un
numéro sémantique). Cohérent avec `--version` qui affiche déjà `"okashi dev"` aujourd'hui dans ce
cas — aucune nouvelle branche de comportement à documenter ailleurs, le hub reflète simplement ce
qui existe déjà.

## Style

Nouveau style dans `styles.go`, à côté de `bannerStyle` :

```go
var versionStyle = lipgloss.NewStyle().
    Foreground(subtle)
```

Discret (couleur `subtle`, déjà utilisée par `statusStyle`, `quoteStyle`), non-gras — ne concurrence
pas visuellement le violet accent (`bannerStyle`) du mot « okashi ».

## Hors périmètre

- Pas de nouveau mécanisme d'injection de version — `main.version` + `-ldflags` existants suffisent.
- Pas de champ `model.version` — la variable globale est lue directement dans `homeContent()`.
- La fonction `bannerView()` (`styles.go:60-62`) est orpheline (aucun appelant dans le code) — ce
  changement ne la touche pas ; elle n'est pas dans le chemin de rendu réel du hub.
- Pas de logique pour distinguer un build Homebrew d'un build manuel autre que la valeur de
  `version` elle-même (déjà le comportement actuel de `--version`).

# Interface en français — Sous-projet 1 : labels, menus, aide

**Date** : 2026-08-01
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

## Contexte

L'interface de forkashi est entièrement en anglais (labels de menus, aide, messages de statut, écran d'accueil, onglets d'inspecteur). Le design v1 du fork (`docs/superpowers/specs/2026-08-01-forkashi-v1-design.md`, §6 « Hors périmètre v1 ») avait explicitement écarté ce sujet de la première itération. Il est maintenant temps de le traiter.

Une exploration du code a quantifié le périmètre réel : **plusieurs centaines de chaînes littérales anglaises** (~470-515 uniques) réparties sur une douzaine de fichiers Go, sans système d'internationalisation existant (traduction en dur, cohérent avec le choix d'un fork mono-langue). Ce volume correspond à plusieurs jours de travail, pas un chantier ponctuel — d'où la décision de découper en sous-projets indépendants plutôt qu'un seul gros chantier.

Ce document couvre le **premier sous-projet** : les labels, menus, et texte d'aide statiques. Les sous-projets suivants (non spécifiés ici) couvriront :
- Le contenu narratif du projet démo « The Lighthouse » (texte littéraire, nature différente de la traduction d'UI).
- Les dépendances anglo-centrées qui limitent l'onglet « Analysis » (`stopWords`, tagger POS `prose/v2`) — chantier technique, pas de la traduction de chaînes.
- La gestion correcte des accords pluriels français dans les messages à interpolation — raffinement ultérieur, une fois le gros du texte déjà en français.

## 1. Périmètre

Remplacement direct (pas de système i18n) des chaînes littérales anglaises visibles à l'utilisateur par leur équivalent français, dans les fichiers suivants (identifiés par l'exploration comme contenant le plus de texte UI) :

- `main.go` — bloc d'aide (F1/?), messages de statut de l'app (save, undo, typewriter, sprint...)
- `home.go` — écran d'accueil (sections PROJECTS/FILES/FOLDERS/RECENT/PINNED/LIBRARY/OTHER)
- `corkboard.go` — labels d'aide contextuelle, messages de confirmation
- `outline.go` — labels d'aide contextuelle
- `inspector.go` — onglets (Words/Outline/Goals/Analysis) et en-têtes de section restants en anglais (les labels du score Kandel-Moles sont déjà en français depuis un sous-projet précédent)
- `mover.go`, `search.go`, `snapshots.go`, `properties.go`, `notes.go`, `goals.go` — messages de statut et prompts

Les messages construits par interpolation (`fmt.Sprintf`) sont traduits **sans gérer l'accord singulier/pluriel** pour l'instant (ex. « 1 correspondances » accepté temporairement, malgré l'incorrection grammaticale) — cohérent avec la décision de préférer un français partout, quitte à une approximation grammaticale ponctuelle, plutôt qu'un mélange anglais/français.

**Hors périmètre de ce sous-projet** : contenu du projet démo, `stopWords`/tagger POS anglo-centrés, gestion fine des accords pluriels, `README.md`, texte `usage` du CLI hors TUI.

## 2. Raccourcis clavier

Les combinaisons physiques (`ctrl+n`, `ctrl+e`, `ctrl+g`, etc.) restent strictement identiques — seul le texte descriptif qui les accompagne dans les labels d'aide/statut est traduit.

Les mnémoniques à une lettre basés sur un mot anglais (`c`=corkboard, `r`=rename/retitle, `a`=add, `x`=remove, `d`=duplicate, `n`=new/note, `m`=manuscript/read) **gardent leur touche physique actuelle**, même si la lettre ne correspond plus visuellement au mot français affiché à côté (ex. la touche `r` reste `r` même si son label devient « renommer »). C'est un compromis assumé : préserver les automatismes déjà acquis avec l'app plutôt que de rouvrir un chantier de réassignation de touches (risque de collisions, complexité largement supérieure au gain).

## 3. Rappel de la touche d'aide

Le rappel de la touche d'aide (vu actuellement sur l'écran d'accueil sous la forme « F1 · ? keybindings ») doit être vérifié et, si nécessaire, ajouté de façon cohérente sur les écrans principaux où il manquerait (éditeur, sidebar, corkboard, outline) — traduit en français, dans le même style que le reste des labels courts de statut.

## 4. Méthode de traduction

Vu le volume (plusieurs centaines de chaînes sur ~12 fichiers), le plan d'implémentation découpe le travail **fichier par fichier** — chaque fichier constitue une tâche indépendante et vérifiable (build propre + vérification visuelle manuelle que rien n'est cassé dans le rendu), suivant le pattern déjà établi dans les sous-projets précédents de ce fork. L'ordre des tâches priorise les fichiers les plus visibles au quotidien (`main.go`, `home.go`) avant les fichiers de fonctionnalités plus secondaires.

## Fichiers concernés

`main.go`, `home.go`, `corkboard.go`, `outline.go`, `inspector.go`, `mover.go`, `search.go`, `snapshots.go`, `properties.go`, `notes.go`, `goals.go`.

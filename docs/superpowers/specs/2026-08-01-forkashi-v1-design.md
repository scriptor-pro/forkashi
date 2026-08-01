# forkashi v1 — Fork français d'okashi

**Date** : 2026-08-01
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

## Contexte

[okashi](https://github.com/snackztime/okashi) est une application d'écriture en terminal (TUI), écrite en Go avec le framework Bubble Tea, pour rédiger des manuscrits longs en Markdown pur. Le projet est jeune (créé le 19/06/2026), sous licence MIT, avec un seul mainteneur (Michael Pentz). Il propose déjà : sidebar de projet, corkboard, snapshots/versions, buts d'écriture avec burndown, heatmap, export RTF/PDF/DOCX, correction orthographique et un correcteur grammatical heuristique — le tout entièrement câblé pour l'anglais (dictionnaire Hunspell `en_US`, règles grammaticales regex anglaises en dur).

Ce document définit le périmètre de **forkashi**, un fork personnel adaptant okashi à un usage francophone, avec trois axes principaux : correction orthographique/grammaticale française, buts d'écriture agrégés au niveau du projet entier, et export au format `.odt`.

Une issue a déjà été ouverte sur le dépôt d'origine ([snackztime/okashi#1](https://github.com/snackztime/okashi/issues/1), 31/07/2026) annonçant cette intention de fork.

## 1. Mise en place du dépôt

- Fork GitHub public de `snackztime/okashi` vers le compte GitHub de l'auteur, renommé **`forkashi`**.
- Cloné dans `/home/Baudouin/Documents/Projets/forkashi`.
- Remote `upstream` conservé (`git remote add upstream https://github.com/snackztime/okashi.git`) pour permettre un suivi ponctuel des évolutions d'okashi, sans obligation de resynchronisation régulière — le projet étant trop jeune pour évaluer une cadence de mises à jour future.
- Le binaire compilé est renommé `forkashi`.

## 2. Correction orthographique française

- Remplacement du dictionnaire Hunspell embarqué :
  - `assets/en.aff` / `assets/en.dic` (anglais, SCOWL `en_US`) → `assets/fr.aff` / `assets/fr.dic` (français).
  - Source : [Dicollecte](https://codeberg.org/dicollage/dictionnaires), récupéré directement (pas via l'emballage extension LibreOffice, qui redistribue le même contenu avec une friction d'extraction supplémentaire).
  - Variante retenue : **« toutes variantes »** (orthographe classique et rectifications de 1990 acceptées simultanément comme correctes) — pas de réglage utilisateur pour choisir entre les deux en v1.
  - Licence : MPL 2.0. Le fichier `assets/en.LICENSE` est remplacé par la licence MPL 2.0 correspondante.
- Le moteur `gospell` (`spell.go`) est Hunspell-générique : aucun changement de dépendance Go n'est nécessaire, seuls les chemins d'embed et les éventuels messages/UI dans `spell.go` changent.
- Le dictionnaire personnel (`~/.config/okashi/dictionary.txt`, ajout via `ctrl+a`) continue de fonctionner à l'identique, sans changement de mécanisme.
- **Hors périmètre v1** : sélection dynamique de langue ou réglage classique/réforme-1990 (l'architecture actuelle ne supporte qu'une seule langue embarquée au build ; ce n'est pas un besoin exprimé pour l'instant).

## 3. Correction grammaticale française (Grammalecte)

- Le correcteur grammatical Tier 1 actuel (`grammar.go`), basé sur des règles regex anglaises en dur (a/an, « could of », locutions figées anglaises...), est **désactivé** — ses règles ne sont pas transposables au français par simple traduction.
- Un nouveau backend, `grammar_grammalecte.go`, implémente l'interface `grammarChecker` déjà définie dans `grammar_backend.go` (aujourd'hui utilisée pour le backend Tier 2 optionnel macOS/Apple Intelligence).
- Ce backend est un **client HTTP** vers un serveur [Grammalecte](https://www.grammalecte.net/) local (mode serveur du projet, port local), choisi plutôt que LanguageTool pour sa spécialisation française fine (accords, conjugaison, typographie FR) et son empreinte plus légère (Python seul, vs Docker+JVM pour LanguageTool). Antidote a été écarté : pas de version native Linux et nécessite un abonnement actif.
- **Cycle de vie du serveur (v1)** : géré **manuellement** par l'utilisateur (lancé en amont, éventuellement via un service systemd utilisateur pour persistance). okashi se contente de s'y connecter s'il est disponible.
- **Comportement en cas d'indisponibilité** : si le serveur Grammalecte ne répond pas (non lancé, port fermé, timeout), okashi désactive silencieusement la vérification grammaticale sans planter — même philosophie best-effort que le backend Tier 2 existant.
- Le rendu visuel (soulignement vert, suggestions) reste inchangé, le système de `Decoration` dans `internal/textarea` étant déjà indépendant de la langue.
- **Amélioration future notée (hors v1)** : lancement/arrêt automatique du serveur Grammalecte par okashi lui-même (gestion de sous-processus, health check, cleanup à la fermeture).

## 4. Buts d'écriture agrégés au niveau du projet

- Le concept `ProjectGoal` (`goals.go`) existe déjà et s'applique au total de mots du dossier-projet entier, pas fichier par fichier — ce qui correspond déjà au besoin exprimé (ex. objectif de 500 mots atteint via 260 mots dans un fichier + 270 dans un autre).
- Travail à réaliser : **vérifier et corriger si nécessaire** la fonction de comptage de mots du projet (`computeProjStats`, dans `manuscript.go`) pour qu'elle parcoure bien tous les fichiers `.md` pertinents du dossier-projet, et non uniquement les chapitres listés dans `manifest.json`.
- Règle par défaut retenue : inclure tous les fichiers `.md` du projet dans le comptage, à l'exception de ceux placés dans un dossier `Resources/` (ou équivalent déjà exclu de l'export) — à ajuster à l'usage si cette règle ne correspond pas à l'attente réelle une fois testée.
- Aucun changement de stockage nécessaire : `goals.json` reste keyé par chemin de projet.

## 5. Export au format .odt

- Nouveau fichier `export_odt.go`, suivant le patron déjà éprouvé par `export_docx.go` (génération OOXML manuelle sans dépendance externe, via `archive/zip` de la stdlib).
- ODT (OpenDocument Text) étant également un format zip+XML, la même approche s'applique : génération de `mimetype`, `META-INF/manifest.xml`, `content.xml`, `styles.xml`.
- Consomme le même AST intermédiaire déjà partagé par RTF/PDF/DOCX (`export_ast.go` : `ManuscriptDoc`/`Section`/`Block`/`Run`) — aucune duplication de la logique de parsing Markdown (goldmark).
- Branché dans `runExport` (`export.go`) comme format de sortie supplémentaire, aux côtés de RTF/PDF/DOCX.
- **Portée** : au choix, comme les formats existants — document courant seul (invocation depuis l'éditeur) ou manuscrit entier compilé (invocation depuis le corkboard/outline, tous les chapitres du manifest concaténés).
- Sortie : `<projet>/export/<slug>.odt`, écriture atomique (réutilise `atomicWrite`).

## 6. Hors périmètre v1

Explicitement reporté, pour rester focalisé sur les trois axes demandés :

- Réglage classique/réforme-1990 pour l'orthographe (dictionnaire unique "toutes variantes" en v1).
- Lancement/arrêt automatique du serveur Grammalecte par okashi.
- Intégration Antidote (non viable sous Linux, licence payante).
- Traduction de l'interface elle-même (menus, labels, aide) — reste en anglais ; seuls le contenu écrit et les fonctionnalités (buts, export) sont adaptés.

Ces points restent documentés ici comme pistes futures, à réévaluer à l'usage.

## Fichiers clés identifiés pour l'implémentation

- `spell.go`, `assets/en.aff`, `assets/en.dic`, `assets/en.LICENSE`, `dict_test.go` — orthographe
- `grammar.go`, `grammar_backend.go`, `grammar_test.go` — grammaire
- `goals.go`, `goals_test.go`, `manuscript.go` (comptage de mots) — buts d'écriture
- `export.go`, `export_ast.go`, `export_docx.go`, `export_docx_test.go`, `docs/superpowers/plans/2026-07-04-docx-export.md` (gabarit de conception pour l'ODT) — export
- `CLAUDE.md` — à lire avant toute modification : documente les invariants d'architecture et les contrats partagés avec l'app macOS compagnon (notamment `manifest.json`), que ce fork personnel s'autorise à ne pas respecter si nécessaire.

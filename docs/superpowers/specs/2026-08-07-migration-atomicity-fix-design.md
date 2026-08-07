# Corriger l'atomicité et la visibilité d'erreur de la migration v1→v2 — Design

> Découvert en debug 2026-08-07 : le projet réel `Jour+` de l'utilisateur était bloqué dans une
> boucle de migration infinie. Cause racine tracée, projet réparé manuellement (voir le fil de
> conversation), ce document couvre uniquement la correction du bug dans le code d'okashi pour
> empêcher la récidive.

## Contexte / bug observé

`needsMigration()` (`migration.go:47-50`) détecte un manifest `schemaVersion == 1` et propose la
migration à chaque lancement d'okashi tant que le manifest reste en v1. L'utilisateur rapporte que
le prompt de migration revient indéfiniment malgré des confirmations répétées — comme si la
conversion n'était "jamais enregistrée, définitive, complète ou correcte".

**Cause racine tracée sur le projet réel `Jour+`** :

1. `migrateV1ToV2` (`migration.go:150-169`) itère sur `plan.moves` et déplace chaque fichier avec
   `os.Rename`, **avant** de savoir si tous les déplacements vont réussir. L'écriture du manifest
   v2 (`writeManifest`) n'a lieu qu'**après** la boucle complète, une seule fois.
2. Sur `Jour+`, le manifest v1 listait `"Bureau-ONU.mk"` (faute de frappe — le vrai fichier est
   `Bureau-ONU.md`). Les 3 premiers items de la boucle (`Annonce.md`, `Attribution.md`,
   `01-untitled.md`) se sont déplacés avec succès dans leurs nouveaux dossiers ; le 4e
   (`Bureau-ONU.mk`) a fait échouer `os.Stat` (le fichier n'existe pas sous ce nom), stoppant la
   boucle et l'écriture du manifest.
3. Le manifest v1 original — qui référence des fichiers déjà déplacés — reste donc en place sur
   disque, inchangé.
4. `confirmMigration()` (`main.go:142-156`) avale l'erreur de retour de `migrateV1ToV2` avec
   `_ = migrateV1ToV2(...)` (ligne 148) : aucune indication n'est donnée à l'utilisateur que la
   migration a échoué.
5. Au lancement suivant, `needsMigration` redétecte le v1 intact, et le cycle recommence à
   l'identique — échec silencieux, prompt qui revient, sans jamais indiquer pourquoi.

## Corrections

Deux changements indépendants, tous deux dans le chemin de migration existant — aucun nouveau
composant, aucun changement de schéma.

### 1. Validation en amont dans `migrateV1ToV2` (`migration.go`)

Avant tout `os.Rename`, une première passe sur `plan.moves` vérifie que chaque `fromFile` existe
(`os.Stat`). Si un seul manque, la fonction retourne l'erreur immédiatement, **sans avoir modifié
le disque** — le manifest v1 et tous les fichiers restent exactement à leur emplacement d'origine.
Ce n'est qu'une fois cette validation complète que la boucle de déplacement s'exécute ; elle ne
peut alors plus échouer sur un fichier manquant. Un échec résiduel dans cette seconde boucle (ex.
erreur de permissions sur `MkdirAll`/`Rename`, bien plus rare) conserve le comportement actuel :
arrêt immédiat, pas d'écriture du manifest, cohérent avec le style "best-effort, ne jamais
deviner" documenté ailleurs dans le projet.

Pas de tentative de migration partielle (option écartée à la conception) : soit tout migre, soit
rien ne bouge.

### 2. Erreur visible dans `confirmMigration` (`main.go`)

`confirmMigration()` capture l'erreur retournée par `migrateV1ToV2` et, si non-nil, l'écrit dans
`m.status` — la barre de statut déjà utilisée pour les erreurs transitoires ailleurs dans le
fichier (ex. `"échec de la vérification grammaticale"`). Le message inclut le nom du fichier
source en cause quand `migrateV1ToV2` peut l'identifier (l'erreur retournée par la validation en
amont porte déjà `mv.fromFile`, donc `%w`-wrappée elle est directement utilisable).

Le comportement de navigation reste identique à aujourd'hui : `m.migrationPending` repasse à
`nil`, l'écran retourne à `screenWriting` — seule différence, `m.status` porte maintenant le
message d'erreur au lieu d'être vide. L'utilisateur peut alors corriger manuellement (renommer le
fichier ou éditer le manifest) puis relancer okashi pour retenter — pas de nouveau flux de retry
automatique, pas de nouvel écran.

### Hors périmètre

- Pas de migration partielle / option "continuer malgré les items en échec".
- Pas de changement à `migrationConfirmView` (l'écran de plan affiché avant confirmation reste
  identique).
- Pas de détection ou correction automatique des extensions de fichier erronées dans le manifest
  v1 (le cas `.mk` vs `.md` de `Jour+` était spécifique à ce projet, corrigé manuellement).
- Pas de nouveau mécanisme de retry automatique en session.

## Tests

- `migrateV1ToV2` : cas où un `fromFile` manque parmi plusieurs items — vérifier qu'aucun fichier
  n'est déplacé (tous restent à leur emplacement d'origine) et que le manifest n'est PAS réécrit
  (reste en v1, contenu identique à l'entrée).
- `migrateV1ToV2` : cas nominal (tous les fichiers présents) — inchangé, doit toujours réussir et
  écrire le manifest v2.
- `confirmMigration` (ou un test au niveau `model` équivalent) : sur une migration en échec,
  `m.status` contient le message d'erreur (et éventuellement le nom du fichier fautif) ;
  `m.migrationPending` repasse à `nil` ; `m.screen == screenWriting`.

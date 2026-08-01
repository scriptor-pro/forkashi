# Lisibilité adaptée au français

**Date** : 2026-08-01
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

## Contexte

L'inspecteur de forkashi affiche un panneau READABILITY avec deux métriques héritées telles quelles d'okashi (anglais) :

- **Reading time** — `mots × 60 / 238`, où 238 est la vitesse de lecture silencieuse moyenne d'un adulte anglophone (méta-analyse Brysbaert 2019, 190 études, 17 887 participants — source solide mais spécifiquement anglophone).
- **Avg sentence** — moyenne ± écart-type de la longueur des phrases en mots, où les phrases sont découpées par la regex `[.!?]+` (`sentenceStats`, `inspector.go`).

Trois défauts pour un usage français, identifiés lors d'une session de test manuel du fork :

1. Le découpage en phrases sur-compte en présence d'abréviations françaises (« M. Dupont » → 2 phrases au lieu d'1) et d'ellipses (« … » ou « ... » → jusqu'à 3 séparateurs).
2. La vitesse de 238 mots/minute n'est pas calibrée pour le français.
3. Il n'existe aucun score de lisibilité global reconnu — seule une mesure de variance stylistique (moyenne±écart-type), qui n'indique pas la difficulté de lecture en tant que telle.

Ce document définit l'adaptation de ces trois points, en préservant l'esprit heuristique (pas de vrai NLP) déjà en place dans le code existant.

## 1. Découpage en phrases plus robuste

Remplacement du split naïf `sentenceSplitRe.Split(text, -1)` par une fonction dédiée qui :

- Ne considère pas un point comme fin de phrase s'il suit une abréviation française courante de la liste : `M.`, `Mme.`, `Mlle.`, `Dr.`, `etc.`, `cf.`, `p.`, `ex.`, `ch.`, `art.`, `vol.`, `éd.`, `trad.`, `av.`, `apr.`, `J.-C.`
- Traite une séquence de points consécutifs (`...`) ou le caractère ellipse unique (`…`) comme **un seul** séparateur de fin de phrase, pas un par caractère.
- Reste une heuristique par liste fermée et regex, cohérente avec le niveau d'approximation déjà accepté dans `sentenceStats` — pas un tokenizer de phrases linguistiquement complet (pas de gestion des citations imbriquées, des points d'abréviation en fin de phrase réelle type « ...ainsi de suite, etc. », etc.).

## 2. Vitesse de lecture ajustée

Remplacement de la constante `238` par `210` (mots/minute), estimation médiane d'une fourchette de 180-230 mpm trouvée pour la lecture silencieuse en français. **Note de fiabilité** : contrairement au 238 wpm anglophone (méta-analyse Brysbaert 2019, solidement sourcée), cette valeur française n'a pas de source aussi rigoureuse — à documenter comme telle dans le code (commentaire explicite), et à ajuster si une source plus fiable est trouvée plus tard.

## 3. Score de lisibilité Kandel-Moles

Ajout d'un score de lisibilité reconnu pour le français, **en complément** de la mesure moyenne±écart-type existante (qui reste affichée — elle informe sur la régularité stylistique, le score Kandel-Moles sur la difficulté globale ; les deux sont complémentaires, pas redondants).

**Formule** (Kandel & Moles, 1958, adaptation française de Flesch Reading Ease) :
```
Score = 207 − 1,015 × (mots / phrases) − 73,6 × (syllabes / mots)
```

**Comptage de syllabes** : heuristique par groupes de voyelles consécutifs (`a, e, i, o, u, y` + voyelles accentuées `é, è, ê, à, â, ù, û, î, ï, ô, œ, æ`), avec une règle de e muet final : un mot se terminant par un `e` non accentué ne compte pas ce e comme groupe de voyelle terminal si le mot contient déjà au moins un autre groupe vocalique. Reste un proxy statistique, pas une analyse phonétique — cohérent avec l'approche standard de Flesch/Kandel-Moles elle-même (jamais un vrai découpage syllabique linguistique).

**Affichage** : score numérique borné à [0, 100] pour l'affichage (la formule brute peut dépasser ces bornes sur des textes extrêmes, convention standard), accompagné d'une étiquette de niveau sur l'échelle standard à 7 paliers :

| Score | Étiquette |
|---|---|
| 80–100 | Très facile |
| 70–80 | Facile |
| 60–70 | Assez facile |
| 50–60 | Moyen |
| 40–50 | Assez difficile |
| 30–40 | Difficile |
| 0–30 | Très difficile |

Format d'affichage : `<score> · <étiquette>` (ex. « 72 · Facile »), ajouté comme nouvelle ligne dans le panneau READABILITY de l'inspecteur, sous les lignes existantes.

## Hors périmètre

- Pas de vrai tokenizer de phrases (gestion de citations imbriquées, dialogues avec tirets, etc.) — hors de portée d'une heuristique légère.
- Pas de calibration du score Kandel-Moles par genre littéraire (roman vs essai vs poésie) — le score reste générique.
- La vitesse de lecture (210 mpm) reste une estimation à affiner si une source plus rigoureuse est trouvée — non bloquant pour cette itération.

## Fichiers concernés

- `inspector.go` — `sentenceStats`, `sentenceSplitRe`, constante `238`, rendu du panneau READABILITY (autour des lignes 143-144, 262-294, 470-471).

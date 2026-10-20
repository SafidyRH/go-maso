# Go-Maso — Roadmap de développement

> **Vision :** construire une plateforme de supervision centralisée, légère et extensible, destinée aux développeurs et petites équipes qui exploitent plusieurs applications. Depuis une interface unique, ils doivent pouvoir savoir **quelles applications fonctionnent, lesquelles rencontrent des problèmes, depuis quand, avec quel impact et quelles actions entreprendre**.
>
> **Document :** feuille de route fonctionnelle et technique, à faire évoluer au fil du développement.
>
> **Statuts :** `À faire` · `En cours` · `Terminé` · `À vérifier`.

## 1. Positionnement et périmètre

Go-Maso n'est **pas** une copie de GoAccess. GoAccess est centré sur l'analyse des logs d'accès web ; Go-Maso sera un centre de supervision **multi-applications** et **multi-signaux**.

### Trois niveaux de monitoring

1. **Externe — sans instrumentation :** requêtes HTTP(S) régulières, disponibilité, latence, erreurs, santé des endpoints, certificat TLS.
2. **Applicatif — avec instrumentation facultative :** métriques, traces, erreurs et événements métier provenant de SDK, d'API de collecte ou de standards ouverts.
3. **Infrastructure — avec agent facultatif :** CPU, RAM, disque, conteneurs Docker, processus et hôtes.

### Principes structurants

- **Progression par valeur :** livrer un outil utile dès le premier MVP avant d'étendre l'observabilité.
- **Approche agentless-first :** la surveillance de disponibilité fonctionne sans modifier les applications.
- **Interopérabilité :** préférer les formats et protocoles ouverts lorsque c'est pertinent (ex. OpenTelemetry, Prometheus) ; ne pas les implémenter tous au départ.
- **Fiabilité :** conserver un historique, distinguer panne de la cible et panne du système de monitoring.
- **Sécurité :** contrôle d'accès, secrets chiffrés ou gérés via secret manager, validation des URL et protection contre les accès aux réseaux internes (SSRF).
- **Portabilité :** développement local via Docker Compose, déploiement futur sur VPS/containers ; éviter le verrouillage fournisseur.
- **Simplicité d'exploitation :** installation, sauvegarde, restauration et mises à jour documentées.

## 2. Architecture cible (évolutive)

```text
                            ┌──────────────────────────┐
                            │  Interface Web Go-Maso   │
                            │  Vue globale + détail    │
                            └────────────┬─────────────┘
                                         │ HTTPS
                            ┌────────────▼─────────────┐
                            │ API Go (auth, projets,   │
                            │ monitoring, alertes)     │
                            └───────┬─────────┬────────┘
                                    │         │
                        ┌───────────▼─┐   ┌───▼────────────────┐
                        │ PostgreSQL  │   │ Scheduler + workers│
                        │ config,     │   │ checks HTTP/TLS,   │
                        │ historiques │   │ règles et alertes  │
                        └─────────────┘   └───┬────────────────┘
                                               │
                 ┌─────────────────────────────┼────────────────────────────┐
                 │                             │                            │
           Applications                   Collecteur                 Agents / sources
           surveillées                 métriques/logs              optionnelles
           HTTP(S)                    (phase avancée)            (phase avancée)
```

**Architecture de départ recommandée :** monolithe modulaire en Go, avec workers intégrés ou séparables, PostgreSQL, API REST et frontend web. **Ne pas introduire** Kafka, Kubernetes ou une architecture microservices avant d'en avoir un besoin mesuré.

### Modules fonctionnels envisagés

- Identité et accès
- Applications / environnements / endpoints
- Exécution et planification des contrôles
- Historique, agrégations et calcul des indicateurs
- Incidents et alertes
- Collecte des logs, métriques et traces (avancé)
- Agents et intégrations (avancé)
- Tableau de bord, reporting et configuration

## 3. État initial connu

Des travaux précédents ont déjà porté sur le **bootstrap Go**, une **API lancée via `go run ./cmd/api`**, la **connexion PostgreSQL via Docker**, et plusieurs étapes de développement.

> **Important :** le contenu exact du dépôt n'a pas été inspecté pour rédiger cette roadmap. Les éléments déjà implémentés sont donc **à vérifier** ; ne pas considérer automatiquement une phase comme terminée.

**Checklist d'audit initial :**

- [x] Inventorier l'arborescence, les modules et endpoints existants.
- [x] Exécuter les tests et identifier les migrations disponibles.
- [x] Vérifier la configuration PostgreSQL (port Docker configurable via `POSTGRES_PORT`, `5434` par défaut).
- [x] Identifier ce qui est déjà fonctionnel et mettre à jour les statuts ci-dessous.
- [x] Documenter la façon de lancer le projet sous Windows et en CI.

---

## 4. Les phases de réalisation

### Phase 0 — Audit et cadrage du MVP

**Statut :** `Terminé` — audit et cadrage consolidés le 16 octobre 2026  
**Objectif :** fixer un périmètre concret, mesurable et réalisable.

**Travaux :**

- Auditer le code actuel et cartographier les acquis.
- Définir les utilisateurs : développeur individuel d'abord, équipe ensuite.
- Définir les objets métier : `Application`, `Environment`, `Monitor`, `CheckResult`, `Incident`, `AlertRule`, `NotificationChannel`.
- Définir les exigences non fonctionnelles : cadence des contrôles, rétention, concurrence, limites, sécurité.
- Poser les indicateurs : **uptime**, taux de réussite, temps de réponse, dernier contrôle, historique des incidents.
- Préparer une liste de parcours utilisateurs et un backlog priorisé.

**Livrables :** périmètre MVP, diagramme d'architecture initial, modèle métier, backlog, conventions de développement.

**Critères de sortie :** les fonctionnalités du MVP, leurs limites et leurs scénarios de validation sont écrits et approuvés.

### Phase 1 — Socle technique robuste

**Statut :** `Terminé` — socle consolidé et validé le 16 octobre 2026  
**Objectif :** rendre l'application prévisible, testable et simple à exécuter.

**Travaux :**

- Structurer le monolithe modulaire (`cmd/`, `internal/`, migrations, configuration).
- Configurer PostgreSQL, migrations versionnées et contraintes d'intégrité.
- Ajouter validation de configuration, logs structurés, gestion centralisée des erreurs et arrêt gracieux.
- Mettre en place les tests unitaires / intégration, lint, formatage, pipeline CI.
- Ajouter endpoint de santé **de Go-Maso lui-même** (`/healthz`, et readiness si nécessaire).
- Fournir Docker Compose pour le développement ; `.env.example` sans secrets.
- Définir une stratégie de sauvegarde et de restauration de la base.

**Livrables :** API reproductible, migrations, commandes de développement, premiers tests et CI.

**Critères de sortie :** un nouveau développeur peut cloner, démarrer, migrer et tester le projet depuis la documentation.

### Phase 2 — Catalogue des applications et des moniteurs

**Statut :** `Terminé` — catalogue consolidé et validé le 16 octobre 2026  
**Objectif :** permettre d'enregistrer les applications à surveiller.

**Travaux :**

- Créer, consulter, modifier, archiver une application.
- Gérer les environnements (`production`, `staging`, etc.) et leurs endpoints.
- Créer un ou plusieurs moniteurs HTTP(S) par application : URL, méthode autorisée, intervalle, timeout, code attendu.
- Prévoir un statut activé/désactivé et un propriétaire.
- Valider les URLs, limiter les redirections et **bloquer les cibles internes/non autorisées** pour prévenir les SSRF.
- Concevoir des APIs REST documentées (OpenAPI) avec pagination et validation des entrées.

**Livrables :** schéma de données, CRUD des applications/environnements/moniteurs, tests API.

**Critères de sortie :** on peut enregistrer deux applications indépendantes et configurer plusieurs contrôles sur chacune.

### Phase 3 — Moteur de monitoring HTTP(S) **(cœur du MVP)**

**Statut :** `Terminé` — moteur consolidé et validé le 16 octobre 2026  
**Objectif :** exécuter des contrôles fiables et enregistrer leurs résultats.

**Travaux :**

- Construire un ordonnanceur (scheduler) et des workers bornés en concurrence.
- Exécuter des contrôles HTTP(S) avec timeout, suivi des codes HTTP, latence, erreur réseau et horodatage UTC.
- Gérer les intervalles, la reprise après redémarrage, les chevauchements et la limitation des ressources.
- Enregistrer chaque résultat avec une politique de rétention.
- Calculer `UP`, `DOWN`, `DEGRADED` et `UNKNOWN` à partir de règles explicites (ex. échecs consécutifs).
- Éviter qu'un échec isolé ou un redémarrage de Go-Maso soit immédiatement interprété comme une panne applicative.
- Ajouter les tests avec serveurs HTTP simulés (réussite, 500, timeout, TLS invalide, DNS inaccessible).

**Livrables :** scheduler, worker HTTP, stockage des résultats, calcul d'état, tests de résilience.

**Critères de sortie :** les contrôles fonctionnent de manière répétée, leurs résultats sont historisés et l'état est recalculé correctement après une panne et un rétablissement.

### Phase 4 — Dashboard opérationnel **(MVP utilisable)**

**Statut :** `À faire`  
**Objectif :** visualiser en quelques secondes l'état de tous les projets.

**Travaux :**

- Construire un frontend web responsive avec vue globale : applications, état, dernier contrôle, disponibilité et latence.
- Ajouter une fiche par application : environnements, moniteurs, historique et événements récents.
- Afficher une courbe de latence et un graphique de disponibilité sur une période configurable.
- Prévoir filtre, recherche, tri et états de chargement/erreur.
- Actualiser les données par polling simple ; évaluer SSE/WebSocket seulement si nécessaire.
- Rendre les temps de référence et les fuseaux explicites côté interface.

**Livrables :** interface web, pages liste/détail, composants statistiques, intégration API.

**Critères de sortie :** une personne peut ouvrir Go-Maso et identifier immédiatement l'application en panne, puis consulter son historique.

### Phase 5 — Incidents, alertes et notifications **(MVP complet)**

**Statut :** `À faire`  
**Objectif :** être averti sans surveiller constamment le tableau de bord.

**Travaux :**

- Créer automatiquement un incident lors d'une transition vers `DOWN` et le résoudre au rétablissement.
- Définir les seuils (échecs consécutifs, fenêtre d'observation, latence excessive).
- Mettre en place des règles d'alerte par moniteur et une gestion des notifications sans doublons.
- Démarrer avec **un canal simple** (ex. email ou webhook) ; autres canaux ensuite.
- Gérer tentatives de livraison, erreurs, cooldown, acquittement si nécessaire.
- Journaliser les événements et fournir un historique des alertes et des incidents.
- Prévoir un mode maintenance pour éviter les alertes pendant une intervention planifiée.

**Livrables :** moteur de règles, incidents, notifications, configuration depuis le dashboard.

**Critères de sortie :** un service simulé tombe, déclenche une alerte unique selon la règle, puis son rétablissement clôt l'incident et peut envoyer une notification de récupération.

### ✅ Jalon M1 — MVP Go-Maso

Le MVP est prêt lorsque l'utilisateur peut **ajouter plusieurs applications**, configurer des **moniteurs HTTP(S)**, visualiser leur **état et historique**, et recevoir une **alerte en cas de panne**.

**Hors MVP, sauf impératif :** logs centralisés, tracing distribué, monitoring CPU/RAM, multi-tenant complexe, facturation, applications mobiles natives, IA, ingestion massive de données.

---

### Phase 6 — Sécurité, comptes et accès multi-utilisateurs

**Statut :** `À faire`  
**Objectif :** exploiter la plateforme de manière sécurisée avec plusieurs utilisateurs.

**Travaux :**

- Authentification et gestion de sessions/tokens sécurisés.
- Gestion des utilisateurs, organisations/espaces de travail si nécessaires, rôles et autorisations.
- Isolation des ressources entre organisations : requêtes, métriques, alertes, secrets.
- Audit des opérations sensibles et politiques de conservation.
- Protection des endpoints publics : rate limits, contrôle des permissions et entrées non fiables.
- Gestion des secrets des intégrations et chiffrement en transit ; sauvegardes sécurisées.

**Livrables :** IAM, RBAC, tests d'autorisation/isolation, audit.

**Critères de sortie :** un utilisateur non autorisé ne peut ni consulter ni modifier les applications, résultats ou canaux d'alerte d'un autre espace.

**Note de séquencement :** si Go-Maso est exposé publiquement ou utilisé par plusieurs personnes dès le MVP, **déplacer les contrôles d'accès indispensables avant le déploiement du MVP** ; la sécurité n'attend pas cette phase.

### Phase 7 — Monitoring HTTP avancé et reporting

**Statut :** `À faire`  
**Objectif :** transformer les contrôles bruts en indicateurs exploitables.

**Travaux :**

- Vérification de certificat TLS (expiration, erreurs) et surveillance des noms de domaine lorsque pertinente.
- Vérification personnalisée de contenu/réponse (dans des limites sécurisées).
- Mesures p50/p95/p99 de latence avec distinction des périodes et volumes.
- Calcul cohérent d'uptime/downtime, fenêtres de maintenance et objectifs SLO.
- Agrégation des données, rétention configurable, exports CSV/JSON.
- Pages de statut privées ou publiques (optionnelles).

**Livrables :** indicateurs agrégés, rapports, règles avancées.

**Critères de sortie :** disponibilité et statistiques historiques cohérentes, explicables et exportables pour une période donnée.

### Phase 8 — Logs HTTP et erreurs applicatives (inspiration GoAccess)

**Statut :** `À faire`  
**Objectif :** expliquer les incidents par les requêtes, erreurs et événements.

**Travaux :**

- Définir une stratégie d'ingestion : API de collecte, agent, export de logs ou intégration avec un backend existant.
- Commencer par les logs **structurés** (JSON) ; prévoir les formats Nginx/Apache si utile.
- Associer les événements à une application, un environnement et des horodatages normalisés.
- Afficher erreurs HTTP, routes les plus sollicitées, latences et tendances.
- Mettre en place contrôle des volumes, déduplication, quotas et rétention.
- **Masquer ou exclure** secrets, tokens, données personnelles et payloads sensibles.
- Étudier OpenTelemetry Logs ou des solutions spécialisées plutôt que réinventer un moteur de recherche massif.

**Livrables :** pipeline d'ingestion limité, consultation/recherche et vues d'analyse HTTP.

**Critères de sortie :** une erreur HTTP significative est retrouvable et liée à l'application concernée, sans fuite de données sensibles.

### Phase 9 — Métriques applicatives et infrastructure

**Statut :** `À faire`  
**Objectif :** surveiller la santé interne des services et machines.

**Travaux :**

- Définir un contrat de collecte des métriques (format, identité des séries, labels, intervalle).
- Explorer l'intégration d'instruments existants (OpenTelemetry / Prometheus) avant tout SDK spécifique.
- Collecter les métriques de processus/applications : CPU, mémoire, débit, latence, erreurs.
- Créer un agent léger facultatif pour VPS / Docker si les intégrations standards ne suffisent pas.
- Gérer authentification des agents, tamponnement et perte réseau.
- Construire des widgets et règles d'alerte sur métriques ; maîtriser la cardinalité et les coûts de stockage.

**Livrables :** intégrations/agent pilote, visualisations de métriques, alertes basées sur seuils.

**Critères de sortie :** pour une application pilote, Go-Maso montre à la fois la disponibilité externe et les métriques internes pertinentes.

### Phase 10 — Observabilité avancée et corrélation

**Statut :** `À faire`  
**Objectif :** accélérer l'identification de la cause d'un incident.

**Travaux :**

- Étudier le support des traces distribuées **via des backends spécialisés** plutôt qu'un stockage maison immédiat.
- Corréler incident, métriques, erreurs et traces par application/environnement et période.
- Ajouter annotations de déploiement, changements de configuration et événements métier.
- Prévoir l'intégration CI/CD et des webhooks entrants.
- Ajouter tableaux de bord personnalisables et rapports périodiques.

**Livrables :** vues corrélées, intégrations et parcours d'investigation.

**Critères de sortie :** depuis un incident, le développeur peut retrouver les signaux utiles pour formuler une hypothèse de cause racine.

### Phase 11 — Industrialisation et déploiement de production

**Statut :** `À faire`  
**Objectif :** héberger Go-Maso durablement sans compromettre les applications surveillées.

**Travaux :**

- Conteneuriser et documenter l'installation sur VPS / Docker Compose.
- Déployer derrière HTTPS/reverse proxy avec domaines et secrets propres à chaque environnement.
- Mettre en place backups automatiques et **tests réels de restauration**.
- Assurer la supervision de Go-Maso lui-même (depuis une source indépendante autant que possible).
- Tester la charge, la taille des historiques, les migrations, les redémarrages et la reprise des workers.
- Documenter maintenance, upgrade, rollback, gestion des incidents et SLO du service.
- Évaluer le besoin de workers séparés ou d'une base de séries temporelles **seulement si les mesures le justifient**.

**Livrables :** stack déployable, pipeline de livraison, runbook, procédures de backup et restauration.

**Critères de sortie :** Go-Maso survit à un redémarrage, conserve ses données, redémarre ses contrôles et peut être restauré selon une procédure testée.

**Note :** un déploiement de préproduction minimal doit être disponible dès le MVP ; cette phase représente la **maturité opérationnelle**, pas la première mise en ligne.

### Phase 12 — Produit mature et écosystème (optionnel)

**Statut :** `À faire`  
**Objectif :** évoluer vers un outil partageable, communautaire ou commercialisable.

**Possibilités, à choisir selon les retours d'usage :**

- Onboarding guidé, import/export de configuration, templates de moniteurs.
- SDK ou CLI Go-Maso ; API publiques versionnées.
- Organisations, équipes, rôles avancés, quotas et facturation si choix SaaS.
- Catalogue d'intégrations : Slack, Discord, Teams, GitHub, GitLab, etc.
- Sondes depuis plusieurs régions pour distinguer incidents réseau locaux et globaux.
- Haute disponibilité, distribution des contrôles et isolation des charges si nécessaire.
- Plugins communautaires, documentation publique, contribution open source.

**Critères de sortie :** les fonctionnalités retenues correspondent à des usages réels, sont maintenables et disposent d'une documentation et de tests.

---

## 5. Ordre d'exécution conseillé

| Priorité | Phases | Résultat observable |
|---|---|---|
| P0 | 0 → 1 | Projet audité et socle technique stable |
| P0 | 2 → 3 | Applications enregistrées et contrôles HTTP périodiques |
| P0 | 4 → 5 | **MVP : monitoring centralisé + alertes** |
| P1 | 6 (ou plus tôt selon exposition) | Accès sécurisé / équipes |
| P1 | 7 | Monitoring HTTP enrichi et reporting |
| P2 | 8 → 9 | Logs, erreurs et métriques internes |
| P2 | 10 | Corrélation des signaux et investigation |
| Transversal | 11 | Exploitabilité et fiabilité en production |
| Selon adoption | 12 | Produit avancé, intégrations, éventuel SaaS |

**Ne pas attendre la fin d'une phase pour tester.** Les tests, la sécurité, l'observabilité de Go-Maso et les migrations doivent accompagner toutes les phases.

## 6. Backlog prioritaire : la prochaine tranche de travail

Après audit du dépôt, dérouler de préférence les éléments suivants :

- [x] Confirmer le modèle `Application` + `Environment` + `Monitor`.
- [x] Implémenter les migrations et endpoints CRUD manquants.
- [x] Ajouter des tests de validation d'URL et de prévention SSRF.
- [x] Implémenter **un check HTTP manuel** sur un moniteur.
- [x] Persister un `CheckResult` (heure, code, durée, erreur).
- [x] Ajouter un scheduler qui déclenche automatiquement les checks.
- [x] Exposer la dernière mesure et l'état calculé via l'API.
- [ ] Afficher ces données dans une première vue de dashboard.
- [ ] Déclencher une alerte simple en cas de panne confirmée.

**Premier scénario de démonstration :** enregistrer une API NestJS et un site Next.js ; couper volontairement l'un des services ; voir la transition d'état dans Go-Maso ; recevoir une alerte ; redémarrer le service ; constater la récupération et l'historique.

## 7. Décisions techniques à consigner (ADR)

Conserver une courte **Architecture Decision Record (ADR)** pour toute décision difficile à changer :

| Sujet | Proposition initiale | Point de réévaluation |
|---|---|---|
| Organisation backend | Monolithe modulaire Go | Séparation si charge démontrée |
| Données relationnelles | PostgreSQL | Volumétrie et requêtes mesurées |
| Orchestration | Scheduler + workers Go | Multi-instance / HA |
| Communication UI | API REST + polling | Besoin confirmé de streaming |
| Déploiement | Docker Compose sur VPS | Haute disponibilité / scale |
| Notifications | Un canal simple d'abord | Besoins réels des utilisateurs |
| Logs et métriques | Intégration de standards/outils existants | Besoins de stockage / recherche |
| Multi-tenant | Pas de complexité SaaS prématurée | Passage à plusieurs organisations |

## 8. Définition de « terminé » pour chaque fonctionnalité

Une fonctionnalité n'est **terminée** que si :

- [ ] Le comportement attendu est décrit, y compris erreurs et cas limites.
- [ ] L'API et/ou l'interface fonctionnent dans un parcours utilisateur réel.
- [ ] Des tests automatisés couvrent les chemins critiques.
- [ ] Les contrôles de sécurité pertinents sont appliqués.
- [ ] Les migrations et configurations sont reproductibles.
- [ ] Les logs/indicateurs nécessaires au diagnostic sont présents.
- [ ] La documentation a été mise à jour.
- [ ] La fonctionnalité a été validée localement ou en préproduction.

## 9. Indicateurs de réussite du produit

- **Détection :** délai entre panne réelle et état `DOWN` affiché.
- **Fiabilité :** ratio de faux positifs, contrôles manqués, reprises après incident.
- **Utilité :** temps pour identifier l'application en échec et accéder aux détails.
- **Performance :** temps de chargement du dashboard, coût de stockage, durée des checks.
- **Opérations :** succès des sauvegardes/restaurations, stabilité du scheduler.
- **Adoption :** nombre d'applications réellement surveillées et utilisation des alertes.

Ne pas figer de cibles chiffrées arbitraires : les fixer après un premier test de charge et des retours utilisateurs.

## 10. Livrable final attendu

À terme, Go-Maso permettra de :

1. Se connecter à un espace sécurisé.
2. Enregistrer et organiser ses applications (Go, Node/NestJS, Next.js, Django, etc.).
3. Surveiller leur disponibilité et leurs performances sans instrumentation obligatoire.
4. Analyser chaque application dans une page dédiée et consulter son historique.
5. Recevoir des alertes utiles, traçables et configurables.
6. Ajouter, si nécessaire, logs, métriques et monitoring d'infrastructure.
7. Corréler les incidents avec les événements techniques.
8. Héberger la plateforme sur sa propre infrastructure, avec une exploitation documentée.

**Définition de la réussite :** un développeur disposant de plusieurs applications peut ouvrir Go-Maso et répondre rapidement aux questions : **« Qu'est-ce qui ne fonctionne pas ? Depuis quand ? Quelle est la gravité ? Quelles données me permettent de comprendre ? »**

---

*Roadmap vivante — mettre à jour les statuts après chaque itération et conserver les décisions d'architecture importantes.*

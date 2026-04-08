---
layout: default
title: Accueil
nav_order: 1
---

# Open Buro — Dossier Technique : File Picker

## Inter-App Communication via Intents & Capabilities

*Préparé pour le hackathon Open Buro — Avril 2026*

---

## Résumé Exécutif

Open Buro vise à établir un **standard ouvert d'orchestration de services collaboratifs**. Le File Picker est le premier cas concret : permettre à n'importe quelle application (mail, docs, chat, agenda…) de demander à n'importe quel drive (TDrive, Fichier DINUM, Nextcloud…) de présenter une interface de sélection de fichiers, et de recevoir le résultat de manière standardisée.

Conceptuellement, trois couches doivent intervir :

| Couche                  | Rôle                                                         | Exemple                                                             |
| ----------------------- | ------------------------------------------------------------ | ------------------------------------------------------------------- |
| **App cliente**         | Demande une action (ex. « attacher un fichier »)             | Tmail, Docs, Element                                                |
| **Source / Capability** | Expose un service (ex. « je sais présenter un file picker ») | TDrive, Fichier DINUM                                               |
| **Plateforme**          | Met en relation le client et la capability                   | Serveur d'intents minimal, ou configuration statique, ex Stack Cozy |

Ce dossier présente les patterns existants, une comparaison des implémentations, et des scénario possibles pour articuler ces 3 couches pendant le hackathon.

---

## Sommaire

1. [État de l'art : Patterns d'Intents & Capabilities](etat-de-lart/)
   - [Android Intents](etat-de-lart/android-intents.md)
   - [Freedesktop.org (XDG)](etat-de-lart/freedesktop.md)
   - [Cozy Cloud / Twake Workplace](etat-de-lart/cozy-twake.md)
   - [openDesk (ZenDiS)](etat-de-lart/opendesk.md)
   - [Google Picker API](etat-de-lart/google-picker.md)
2. [Tableau comparatif](comparatif.md)
3. [Proposition pour le Hackathon](proposition-hackathon/)
   - [Sémantique du File Picker Intent](proposition-hackathon/semantique-file-picker.md)
   - [Protocole postMessage détaillé](proposition-hackathon/protocole-postmessage.md)
   - [Contournements navigateur](proposition-hackathon/contournements-navigateur.md)
4. [Sujets de réflexion pour les ateliers](sujets-ateliers.md)
5. [Bibliographie](bibliographie.md)
6. [Glossaire](glossaire.md)

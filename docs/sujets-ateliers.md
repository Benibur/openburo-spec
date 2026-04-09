---
layout: default
title: Sujets de réflexion
nav_order: 5
---

# Sujets de réflexion pour les ateliers

[← Proposition Hackathon](proposition-hackathon/) · [Accueil](index.md)

---

Ces questions structurent les discussions techniques du hackathon :

## Sémantique des capabilities dans les manifests des apps

- Quels verbes d'action standardiser au-delà de `PICK` et `SAVE` ? (`VIEW`, `EDIT`, `CREATE`, `SHARE` ?)
- Comment typer les données ? MIME types purs, types domaine (comme `io.cozy.files`), ou un vocabulaire Open Buro dédié ?
- Faut-il supporter des catégories (comme Android) ou le couple action/type suffit-il ?

**Choix pour le techsprint** :
- json
```json
{ 
    id:""
    name:""
    url:""
    version:""
    capabilities:[
        {
            action:"PICK"
            properties:[
                mimeTypes:["*/*"]
            ]
            path:""
        },
        {
            action:"SAVE",
            properties:[
                mimeTypes:["*/*"]
            ],
            path:""
        }
        ]
}

```

## Processus de récupération des capabilities

- Cache côté client ? Durée de vie ?
- Source unique (registre plateforme) ou fédéré (chaque app expose son manifest) ?
- Gestion du multi-capability : que se passe-t-il quand 3 drives déclarent `PICK files` ?
- UX du chooser : l'app cliente gère-t-elle le choix, ou la plateforme fournit-elle un composant ?

**Choix pour le techsprint** :
* l'app cliente indique au sdk l'url où récupérer le tableau des manifest
* `ob_sdk.init({manifests:string|obj})`

## intent mangement : ouverture & initialisation de l'intent

- URL complète dans le registre ou base URL + route ?
- Paramètres passés en query string, dans le hash, ou uniquement via postMessage ?
- iframe vs nouvel onglet vs popup ? Contraintes UX et techniques de chacun
- Taille et positionnement de l'iframe (modale, panneau latéral, plein écran ?)

**Choix pour le techsprint** :
* 
* 

## Communication client & capability

- Protocole postMessage : quels types de messages au-delà de ready/init/done/error ?
- Progress : le service peut-il notifier une progression (upload en cours, etc.) ?
- Sécurité : validation d'origin, protection contre le spoofing de messages
- Timeout : que faire si le service ne répond pas ?

## Cycle de vie du front de l'capability

- Le service fournit-il une UI complète ou un composant embarquable ?
- Responsive : le front du picker doit-il s'adapter à la taille de l'iframe ?
- `intent:resize` : le service peut-il demander un redimensionnement ?
- Theming : le picker doit-il respecter le thème de l'app cliente ?

## Callback & résultat

- Retour par référence (URL/lien de partage) vs retour par valeur (contenu base64) ?
- Pour les liens : qui gère l'expiration du token ? Le drive, selon sa propre stratégie
- Pour le contenu : limite de taille raisonnable pour base64 dans postMessage ?
- Métadonnées minimales dans la réponse : `name`, `mimeType`, `size`, `url` — quoi d'autre ?

## Sécurité (CORS, CSP, et au-delà)

- En production : comment gérer CSP `frame-ancestors` sans tout ouvrir ?
- La plateforme doit-elle fournir une liste blanche de domaines autorisés en iframe ? comment les connaitres ?
- Protection contre le clickjacking quand le picker est en iframe
- Persistent capabilitys : un intent peut-il rester ouvert (ex. sync continue, ou pour ouverture plus rapide) ?

---

[Suivant : Bibliographie →](bibliographie.md)

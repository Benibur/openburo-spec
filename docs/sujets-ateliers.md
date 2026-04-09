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

```json
{ 
    id:""
    name:""
    url:""
    version:""
    capabilities:[
        {
            action:"PICK"
            properties:{
                mimeTypes:["*/*"]
            }
            path:""
        },
        {
            action:"SAVE",
            properties:{
                mimeTypes:["*/*"]
            },
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

## intent mangement : cyble de vie de la capability

- Paramètres passés en query string, dans le hash, ou uniquement via postMessage ?
- iframe vs nouvel onglet vs popup ? Contraintes UX et techniques de chacun
- Taille et positionnement de l'iframe (modale, panneau latéral, plein écran ?)

**Choix pour le techsprint** :
* les paramètres d'init sont envoyés en query string
* iframe
* modal avec une taille fixée
```
?clientUrl= // pour la targetOrigin
&id= // pour éviter des conflits sur plusieurs pickers sur plusieurs onglets qui doit être passé ensuite tous les postMessage
&type= // , separated: permet de choisir si on veut en retour des liens ou des binaires ou les deux
&allowedMimeType // , separated: permet de filtrer les fichiers affichées par le file picker, check mime type on mdn
&multiple=boolean // default = false
```


## Communication client & iframe

- Protocole postMessage : quels types de messages au-delà de ready/init/done/error ?
- Progress : le service peut-il notifier une progression (upload en cours, etc.) ?
- Timeout : que faire si le service ne répond pas ?

**Choix pour le techsprint** :
* pas de communication pendant le cycle de vie autre que le init et callback final

## Callback & résultat

- Retour par référence (URL/lien de partage) vs retour par valeur (contenu base64) ?
- Pour les liens : qui gère l'expiration du token ? Le drive, selon sa propre stratégie
- Pour le contenu : limite de taille raisonnable pour base64 dans postMessage ?
- Métadonnées minimales dans la réponse : `name`, `mimeType`, `size`, `url` — quoi d'autre ?

**Choix pour le techsprint** :
```
{
    status: done,
    id: string,
    // allow multifile selection
    results: [
        // allow to get both type from a file
        {
            name: string
            mimeType: string
            size: number // bytes  
            sharingUrl?: string
            downloadUrl?: string
            payload?: any
        }
    ]

}
```

```
{
    "status": "error",
    id: string,
    "message": string
}
```


## Sécurité (CORS, CSP, et au-delà)

- En production : comment gérer CSP `frame-ancestors` sans tout ouvrir ?
- La plateforme doit-elle fournir une liste blanche de domaines autorisés en iframe ? comment les connaitres ?
- Sécurité : validation d'origin, protection contre le spoofing de messages
- Protection contre le clickjacking quand le picker est en iframe
- Persistent capabilitys : un intent peut-il rester ouvert (ex. sync continue, ou pour ouverture plus rapide) ?

service CSP != iframe CSP


**Choix pour le techsprint** :
* le service doit avoir la liste des URLs qui peuvent l'appeler => v1 hardcodé, v2 manifest de client, vX openburo server
* le client doit avoir la liste des URLS qu'il peut intéger => déduit du manifest
* targetOrigin = le client passe dans la query string d'init l'url du client pour éviter les navigate dans le parent
* clickjacking = pas de soucis si CSP ? à vérifier

**Non priorisé**
Persistent capabilitys: besoin de init en postMessage, pas nécessaire pour le POC

---

[Suivant : Bibliographie →](bibliographie.md)

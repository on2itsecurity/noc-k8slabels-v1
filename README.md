# noc-k8slabels-v1

Kubernetes Informer controller for instant syncing `ip`-`podlabels` dictionary with an Palo Alto Firewall.

`noc-k8slabels-v1` will watch namespaces for recreation/updates/deletion of pods send api calls to a Palo Alto Firewall, making it possible to define Dynamic Address Groups based upon Kubernetes labels. Your Kubernetes cluster needs to be in direct routing mode and should not apply any source NAT.  Code was inspired upon https://github.com/bitnami-labs/kubewatch and http://api-lab.paloaltonetworks.com/registered-ip.html
Inside the cluster it needs a `ServiceAccount` coupled to a `ClusterRole` with:
```
apiGroups: [""]
resources: ["pods"]
verbs: ["get", "watch", "list"]
```
It can be run insite the cluster with a `ServiceAccount` or outside the cluster using the currentContext in ~/.kube/config


## env.ini
`env.ini` needs to be mounted in the root of the container, or use -inifile to define a diffrent path. `env.ini` contains the following:
```
[PAN-FW]
Token=                     - Contains the API key of the palo alto firewall in order to update dynamic registrations
URL=                       - url of firewall(s), separated by a comma (without the /api endpoint) e.g. `https://mypaloaltofw1.on2it.net`
RegisterExpire=            - Amount of seconds the label registration should persist. Should be higher than `FullReSync`

[SYNC]
Namespace=                 - Kubernetes Namespaces to watch, seperated by a `,`. Leave empty for all namespaces
LabelKeys=                 - Labels to sync, specerated by a `,` Leave empty for all labels. e.g.: `k8s-app,app`
FullResync=                - Full sync of current state in seconds. 0 is only startup
```
## Deployment
See deployment directory


## Wish list
This project has still a lot of features to be desired, such as:
 * A better name
 * Better code... if you are not ashamed of your first release, you have shipped to late :)
 * CA check / support
 
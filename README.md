# external-dns-target-admission

Automatically add the ExternalDNS "target" annotation to Kubernetes Ingresses and Istio Gateways

## Why?

[ExternalDNS](https://github.com/kubernetes-sigs/external-dns) is an awesome piece of software that can automatically
provision DNS records that correspond to various Kubernetes resources, such as Ingresses and Istio Gateways.

Each record that is created by ExternalDNS corresponds to a specific host that is specified within one of these resources,
and the public IP address is determined by looking at the external IP address assigned to them, typically by using a Kubernetes
Service with type `LoadBalancer`.

Unfortunately, clusters that run in an on-premise cluster (or a homelab, in my case) can't take advantage of `LoadBalancer`
services. Thus, we must resort to setting the `external-dns.alpha.kubernetes.io/target` annotation on Ingresses and Gateways
that specifies the IP address to use for the DNS A record.

This admission controller automatically adds this annotation to all Ingresses and Gateways for you.

## Install

```bash
helm repo add mrparkers https://mrparkers.github.io/charts
helm install ${releaseName} mrparkers/external-dns-target-admission
```

For more information about installing via Helm, refer to the chart docs [here](https://github.com/mrparkers/charts/tree/master/charts/external-dns-target-admission).

{{ define "post-install.sh.tpl" }}
{{- $name := include "external-dns-target-admission.fullname" . -}}
#!/bin/sh

set -eux

caBundle=$(kubectl get secret -n {{ .Release.Namespace }} {{ $name }}-webhook-certificate -o jsonpath='{.data.ca\.crt}')

cat <<EOF > /tmp/patch.yaml
webhooks:
- name: external-dns-target-admission.parker.gg
  clientConfig:
    caBundle: ${caBundle}
EOF

kubectl patch mutatingwebhookconfiguration {{ $name }} --patch "$(cat /tmp/patch.yaml)"

{{- end }}

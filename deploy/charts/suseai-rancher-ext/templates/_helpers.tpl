{{- define "ui-extension.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride }}
{{- else -}}
{{- include "ui-extension.name" . }}-ext
{{- end -}}
{{- end }}

{{- define "ui-extension.name" -}}
{{- default .Chart.Name .Values.nameOverride -}}
{{- end }}

{{- define "ui-extension.labels" -}}
catalog.cattle.io/ui-extensions-catalog-image: ui-extension-suseai-rancher-ext
app.kubernetes.io/name: {{ include "ui-extension.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

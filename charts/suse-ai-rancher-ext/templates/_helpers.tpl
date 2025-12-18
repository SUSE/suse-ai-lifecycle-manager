{{- define "suse-ai-rancher-ext.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride }}
{{- else -}}
{{- include "suse-ai-rancher-ext.name" . }}
{{- end -}}
{{- end }}

{{- define "suse-ai-rancher-ext.name" -}}
{{- default .Chart.Name .Values.nameOverride -}}
{{- end }}

{{- define "suse-ai-rancher-ext.labels" -}}
catalog.cattle.io/ui-extensions-catalog-image: {{ .Chart.Name }}
app.kubernetes.io/name: {{ include "suse-ai-rancher-ext.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Return the proper Docker Image Registry Secret Names
*/}}
{{- define "suse-ai-rancher-ext.imagePullSecrets" -}}
{{/*
Helm 2.11 supports the assignment of a value to a variable defined in a different scope,
but Helm 2.9 and 2.10 does not support it, so we need to implement this if-else logic.
Also, we can not use a single if because lazy evaluation is not an option
*/}}
{{- if .Values.global }}
{{- if .Values.global.imagePullSecrets }}
imagePullSecrets:
{{- range .Values.global.imagePullSecrets }}
  {{- $imagePullSecrets := list }}
  {{- if kindIs "string" . }}
    {{- $imagePullSecrets = append $imagePullSecrets (dict "name" .) }}
  {{- else }}
    {{- $imagePullSecrets = append $imagePullSecrets . }}
  {{- end }}
  {{- toYaml $imagePullSecrets | nindent 2 }}
{{- end }}
{{- else if .Values.imagePullSecrets }}
imagePullSecrets:
    {{ toYaml .Values.imagePullSecrets }}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Name of the chart.
Uses .Values.nameOverride if set, otherwise falls back to chart name.
*/}}
{{- define "suse-ai-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{/*
Full name of the chart.
Always truncated to 63 chars for Kubernetes compatibility.
*/}}
{{- define "suse-ai-operator.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else }}
{{- include "suse-ai-operator.name" . | trunc 63 | trimSuffix "-" -}}
{{- end }}
{{- end }}

{{/*
Namespace for generated references.
Always uses the Helm release namespace.
*/}}
{{- define "suse-ai-operator.namespaceName" -}}
{{ .Release.Namespace }}
{{- end }}

{{/*
Service name with proper truncation for Kubernetes 63-character limit.
Takes a context with .suffix for the service type (e.g., "webhook-service").
If fullname + suffix exceeds 63 chars, truncates fullname to 45 chars.
*/}}
{{- define "suse-ai-operator.serviceName" -}}
{{- $fullname := include "suse-ai-operator.fullname" .context -}}
{{- if gt (len $fullname) 45 -}}
{{- printf "%s-%s" (trunc 45 $fullname | trimSuffix "-") .suffix | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" $fullname .suffix | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end }}

{{/*
Common labels for Helm charts.
Includes app version, chart version, app name, instance, and managed-by labels.
*/}}
{{- define "suse-ai-operator.labels" -}}
{{- if .Chart.AppVersion -}}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
{{- if .Chart.Version }}
helm.sh/chart: {{ .Chart.Version | quote }}
{{- end }}
app.kubernetes.io/name: {{ include "suse-ai-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
control-plane: controller-manager
{{- end }}

{{/*
Selector labels for matching pods and services.
Only includes name and instance for consistent selection.
*/}}
{{- define "suse-ai-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "suse-ai-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
control-plane: controller-manager
{{- end }}

{{/*
Return the proper Docker Image Registry Secret Names
*/}}
{{- define "suse-ai-operator.imagePullSecrets" -}}
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
{{- else if .Values.manager.imagePullSecrets }}
imagePullSecrets:
    {{ toYaml .Values.manager.imagePullSecrets }}
{{- end -}}
{{- end -}}
{{- end -}}

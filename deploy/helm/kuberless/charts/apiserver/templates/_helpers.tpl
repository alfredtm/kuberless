{{- define "apiserver.fullname" -}}
{{- printf "%s-apiserver" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "apiserver.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: kuberless
app.kubernetes.io/component: apiserver
{{- end }}

{{- define "apiserver.selectorLabels" -}}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: apiserver
{{- end }}

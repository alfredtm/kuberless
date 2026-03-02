{{- define "operator.fullname" -}}
{{- printf "%s-operator" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "operator.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: kuberless
app.kubernetes.io/component: operator
{{- end }}

{{- define "operator.selectorLabels" -}}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: operator
{{- end }}

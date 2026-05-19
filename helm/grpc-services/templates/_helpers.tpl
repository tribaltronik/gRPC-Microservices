{{- define "grpc-services.labels" -}}
app.kubernetes.io/name: {{ .name }}
app.kubernetes.io/instance: {{ .name }}
app.kubernetes.io/version: {{ .version }}
app.kubernetes.io/component: microservice
app.kubernetes.io/part-of: grpc-services
{{- end -}}

{{- define "grpc-services.selectorLabels" -}}
app: {{ .name }}
version: {{ .version }}
{{- end -}}

{{- define "grpc-services.serviceAccountName" -}}
{{ .name }}
{{- end -}}

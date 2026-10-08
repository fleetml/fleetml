{{- define "fleetml.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 50 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 50 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "fleetml.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "fleetml.selector" -}}
app.kubernetes.io/name: {{ .root.Chart.Name }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "fleetml.secretName" -}}
{{- default (printf "%s-secrets" (include "fleetml.fullname" .)) .Values.secrets.existingSecret -}}
{{- end -}}

{{- define "fleetml.image" -}}
{{- printf "%s:%s" .image.repository (default .root.Chart.AppVersion .image.tag) -}}
{{- end -}}

{{- define "fleetml.databaseURL" -}}
{{- if .Values.postgres.enabled -}}
postgres://fleetml:$(POSTGRES_PASSWORD)@{{ include "fleetml.fullname" . }}-postgres:5432/fleetml?sslmode=disable
{{- else -}}
{{- required "postgres.url is required when postgres.enabled=false" .Values.postgres.url -}}
{{- end -}}
{{- end -}}

{{- define "fleetml.natsURL" -}}
{{- if .Values.nats.enabled -}}
nats://{{ include "fleetml.fullname" . }}-nats:4222
{{- else -}}
{{- required "nats.url is required when nats.enabled=false" .Values.nats.url -}}
{{- end -}}
{{- end -}}

{{- define "fleetml.s3Endpoint" -}}
{{- if .Values.seaweedfs.enabled -}}
{{ include "fleetml.fullname" . }}-s3:8333
{{- else -}}
{{- required "s3.endpoint is required when seaweedfs.enabled=false" .Values.s3.endpoint -}}
{{- end -}}
{{- end -}}

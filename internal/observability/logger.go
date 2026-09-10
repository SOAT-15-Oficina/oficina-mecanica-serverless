// Package observability monta o log estruturado desta funcao.
//
// O CONTRATO NAO E DAQUI. Os nomes de campo e de evento sao a taxonomia da
// ADR-0011 (oficina-mecanica-infrastructure, docs/adr/0011-logs-estruturados-com-correlacao.md),
// e sao lidos literalmente pelas metricas de log e pelos alertas do Datadog
// definidos em persistent/datadog_metrics.tf e persistent/datadog_monitors.tf.
// Renomear um campo daqui nao quebra nenhum teste: quebra um painel, em
// silencio. Por isso os nomes sao constantes com um lugar so.
//
// O monolito tem um pacote gemeo, com os mesmos campos e as mesmas constantes.
// Sao dois runtimes diferentes com um formato de log unico -- e o que permite a
// mesma consulta atravessar a Lambda e os pods.
package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
)

// Nome deste servico no campo `service`, e a tag `service` do lado do Datadog.
// O Terraform injeta o mesmo valor em DD_SERVICE (ephemeral/lambda.tf); o
// default existe para o teste e para o docker-compose local, onde nao ha
// Terraform nenhum.
const DefaultService = "auth-lambda"

// version e gravada pelo linker no build do CI:
//
//	-ldflags="-X github.com/.../internal/observability.version=$GITHUB_SHA"
//
// Vem do linker, e nao do ambiente, porque e propriedade do ARTEFATO: uma
// variavel de ambiente pode ser trocada sem trocar o binario, e o campo
// passaria a mentir sobre qual codigo esta rodando -- que e justamente a
// pergunta que ele existe para responder ("essa regressao veio de qual deploy?").
var version = "dev"

// Campos fixos da taxonomia (ADR-0011, secao 3).
const (
	KeyService     = "service"
	KeyEnv         = "env"
	KeyVersion     = "version"
	KeyRequestID   = "request_id"
	KeyRoute       = "route"
	KeyMethod      = "method"
	KeyStatus      = "status"
	KeyDurationMS  = "duration_ms"
	KeyEvent       = "event"
	KeyIntegration = "integration"
	KeyError       = "error"
	KeyUser        = "user"
	KeyRole        = "role"

	// Atributo PADRAO do Datadog para status HTTP, emitido em paralelo a
	// `status`. Nao e redundancia gratuita: no pre-processamento de log JSON o
	// Datadog trata `status` como o NIVEL do log (a mesma posicao de `level`) e
	// consome o atributo. Se isso acontecer, `@status` some do log e o
	// group_by da metrica de latencia fica sem a dimensao; `@http.status_code`
	// continua la, e a correcao vira uma linha no Terraform em vez de um novo
	// deploy da aplicacao.
	KeyHTTPStatusCode = "http.status_code"
)

// Eventos de dominio desta funcao (ADR-0011, secao 4).
//
// O nome vai em `event`, NUNCA no `msg`: `msg` e texto para humano e muda na
// primeira refatoracao de mensagem; `event` e identificador e e o que a metrica
// `oficina.auth_login_failed` conta.
const (
	EventLoginFailed = "auth.login_failed"
)

// Dependencias externas, para o campo `integration` em linhas de ERROR. E por
// ele que a metrica `oficina.integration_error` agrupa o painel de "erros e
// falhas nas integracoes".
const (
	IntegrationRDS = "rds"
)

// Setup monta o logger do processo e o instala como default do slog.
//
// Roda uma vez por container, na inicializacao -- nao por invocacao.
func Setup() *slog.Logger {
	logger := New(os.Stdout)
	slog.SetDefault(logger)
	return logger
}

// New monta o logger com os tres campos que valem para toda linha desta
// funcao. O destino e parametro para o teste conseguir ler o que foi escrito.
func New(w io.Writer) *slog.Logger {
	env := envOr("DD_ENV", "local")
	service := envOr("DD_SERVICE", DefaultService)

	var handler slog.Handler
	if env == "local" {
		// JSON e ilegivel no terminal sem jq, e no desenvolvimento local nao ha
		// Datadog para consumi-lo (ADR-0011, "Consequencias").
		handler = slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
	} else {
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})
	}

	return slog.New(handler).With(
		slog.String(KeyService, service),
		slog.String(KeyEnv, env),
		slog.String(KeyVersion, version),
	)
}

// Version devolve a versao gravada no binario. Existe para o teste conseguir
// afirmar que o campo e emitido sem depender do valor.
func Version() string { return version }

type loggerKey struct{}

// WithLogger guarda o logger JA DECORADO com os campos da requisicao no
// contexto. Todo log de dentro de uma invocacao sai dele, e nunca do default:
// e o que garante que `request_id` acompanha a linha ate o fundo da pilha.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// FromContext devolve o logger da requisicao, ou o default quando nao houver
// (inicializacao do container, teste que nao passou por WithLogger).
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}

// Event, Integration e Err existem para o nome do campo aparecer uma vez so no
// codigo de chamada. `slog.String("event", ...)` espalhado e onde o erro de
// digitacao vira painel vazio.
func Event(name string) slog.Attr { return slog.String(KeyEvent, name) }

func Integration(name string) slog.Attr { return slog.String(KeyIntegration, name) }

func Err(err error) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}
	return slog.String(KeyError, err.Error())
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

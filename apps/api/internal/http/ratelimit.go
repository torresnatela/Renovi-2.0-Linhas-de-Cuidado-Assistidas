package http

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/renovisaude/renovi-care/internal/controllers"
)

// rateLimitByIP limita requisições por IP de origem.
//
// Por que existe: sem senha errada custar nada, o login é alvo de força bruta —
// a senha é o único fator (não há verificação por WhatsApp ainda). No cadastro,
// segura a varredura de CPFs.
//
// Limitações conhecidas, aceitáveis no piloto:
//
//   - Estado em memória: com mais de uma instância, cada uma tem sua própria
//     conta. Para o piloto (instância única) serve; se escalar horizontalmente,
//     isto precisa ir para o Redis.
//   - O mapa tem TETO. O reaper sozinho não bastava: ele passa a cada minuto, e
//     entre duas passadas um atacante rotacionando IP crescia a memória sem
//     limite.
//   - Por IP: NAT corporativo faz muitos colaboradores compartilharem um IP.
//     Por isso o burst é generoso — a trava é contra script, não contra gente.
func rateLimitByIP(burst int, perSecond float64) func(http.Handler) http.Handler {
	return rateLimitBy(func(r *http.Request) string { return "ip:" + clientIP(r) }, burst, perSecond)
}

// rateLimitByAccount limita por CONTA da sessão, não por IP.
//
// Para uma rota autenticada e cara (o agendamento fala com a DAV por ~30s e faz
// escrita não idempotente), a conta é a chave certa: é justa sob NAT corporativo
// (colaboradores não dividem o mesmo balde) e — o que mais importa — NÃO é
// spoofável, ao contrário do IP derivado de header (ver o risco do RealIP em
// docs/DECISOES.md). Exige que RequireSession rode ANTES deste middleware, o que
// o router garante.
func rateLimitByAccount(burst int, perSecond float64) func(http.Handler) http.Handler {
	return rateLimitBy(func(r *http.Request) string {
		if acc, ok := controllers.AccountFrom(r.Context()); ok {
			return "acct:" + acc.ID.String()
		}
		// Sem sessão aqui não deveria acontecer (RequireSession roda antes). Cai no
		// IP: pior, mas melhor que não limitar nada.
		return "ip:" + clientIP(r)
	}, burst, perSecond)
}

// rateLimitBy é o núcleo: limita por uma chave qualquer extraída da requisição.
func rateLimitBy(key func(*http.Request) string, burst int, perSecond float64) func(http.Handler) http.Handler {
	l := &ipLimiter{
		limiters:   make(map[string]*visitor),
		burst:      burst,
		rate:       rate.Limit(perSecond),
		maxEntries: maxTrackedIPs,
	}
	go l.reapForever()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(key(r)) {
				controllers.WriteProblem(w, http.StatusTooManyRequests, "tentativas demais",
					"aguarde alguns instantes e tente novamente")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// maxTrackedIPs é o teto de IPs vigiados ao mesmo tempo. ~64k entradas de
// algumas dezenas de bytes: caro o bastante para não passar despercebido, barato
// o bastante para não ser o gargalo.
const maxTrackedIPs = 65536

type ipLimiter struct {
	mu         sync.Mutex
	limiters   map[string]*visitor
	burst      int
	rate       rate.Limit
	maxEntries int
}

func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	v, ok := l.limiters[ip]
	if !ok {
		// No teto, descarta o mais antigo para abrir espaço. Preferimos perder a
		// memória de um IP ocioso a estourar a memória do processo.
		if l.maxEntries > 0 && len(l.limiters) >= l.maxEntries {
			l.evictOldestLocked()
		}
		v = &visitor{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.limiters[ip] = v
	}
	v.lastSeen = time.Now()
	return v.limiter.Allow()
}

// evictOldestLocked remove a entrada vista há mais tempo. Exige l.mu.
func (l *ipLimiter) evictOldestLocked() {
	var oldestIP string
	var oldest time.Time
	for ip, v := range l.limiters {
		if oldestIP == "" || v.lastSeen.Before(oldest) {
			oldestIP, oldest = ip, v.lastSeen
		}
	}
	if oldestIP != "" {
		delete(l.limiters, oldestIP)
	}
}

// reapForever descarta IPs ociosos, para o mapa não segurar memória de quem já
// foi embora. O teto (evictOldestLocked) é que impede o crescimento sem limite;
// este laço só recupera espaço no caso normal.
func (l *ipLimiter) reapForever() {
	for range time.Tick(time.Minute) {
		l.mu.Lock()
		for ip, v := range l.limiters {
			if time.Since(v.lastSeen) > 10*time.Minute {
				delete(l.limiters, ip)
			}
		}
		l.mu.Unlock()
	}
}

// clientIP: o middleware.RealIP do chi já normalizou os cabeçalhos de proxy
// antes daqui.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

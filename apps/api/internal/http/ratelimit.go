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
//   - Por IP: NAT corporativo faz muitos colaboradores compartilharem um IP.
//     Por isso o burst é generoso — a trava é contra script, não contra gente.
func rateLimitByIP(burst int, perSecond float64) func(http.Handler) http.Handler {
	l := &ipLimiter{
		limiters: make(map[string]*visitor),
		burst:    burst,
		rate:     rate.Limit(perSecond),
	}
	go l.reapForever()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(clientIP(r)) {
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

type ipLimiter struct {
	mu       sync.Mutex
	limiters map[string]*visitor
	burst    int
	rate     rate.Limit
}

func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	v, ok := l.limiters[ip]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.limiters[ip] = v
	}
	v.lastSeen = time.Now()
	return v.limiter.Allow()
}

// reapForever descarta IPs ociosos. Sem isto o mapa cresce sem teto e vira um
// vetor de exaustão de memória — basta o atacante variar o IP de origem.
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

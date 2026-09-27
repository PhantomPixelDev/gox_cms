package handlers

import (
	"net"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
)

// loginAttempt tracks consecutive failures in one window.
type loginAttempt struct {
	fails     int
	firstSeen time.Time
}

// The throttle is process-local by design (see below). Three buckets: per IP,
// per account name, and per *remote address*. The remote-address bucket cannot
// be forged: c.IP() is taken from X-Forwarded-For for any address inside
// server.trusted_proxies, so an attacker who rotates the header would otherwise
// get an unlimited attempt budget. c.Context().RemoteAddr() is the socket peer
// and is what the app actually listens on.
var loginThrottle struct {
	sync.Mutex
	byIP      map[string]*loginAttempt
	byAccount map[string]*loginAttempt
	byRemote  map[string]*loginAttempt
	ops       int
}

func init() {
	loginThrottle.byIP = make(map[string]*loginAttempt)
	loginThrottle.byAccount = make(map[string]*loginAttempt)
	loginThrottle.byRemote = make(map[string]*loginAttempt)
}

// NOTE: with server.prefork (multiple processes) each process keeps its own
// counters. Prefork is documented as incompatible with SQLite and off by
// default; single-process is the supported deployment.

// dummyPasswordHash absorbs a bcrypt comparison when the username does not
// exist, so unknown-user and wrong-password logins take the same time.
var dummyPasswordHash []byte

func init() {
	// Same cost as real password checks: unknown-user logins must not fail
	// visibly faster than wrong-password ones.
	hash, err := bcrypt.GenerateFromPassword([]byte("goxcms-dummy-password"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	dummyPasswordHash = hash
}

func loginThrottleMaxAttempts() int {
	if n := viper.GetInt("auth.login_max_attempts"); n > 0 {
		return n
	}
	return 10
}

func loginThrottleWindow() time.Duration {
	if m := viper.GetInt("auth.login_window_minutes"); m > 0 {
		return time.Duration(m) * time.Minute
	}
	return 5 * time.Minute
}

func bucketBlocked(bucket map[string]*loginAttempt, key string, max int, window time.Duration, now time.Time) bool {
	a, ok := bucket[key]
	if !ok {
		return false
	}
	if now.Sub(a.firstSeen) > window {
		delete(bucket, key)
		return false
	}
	return a.fails >= max
}

func bucketRecord(bucket map[string]*loginAttempt, key string, window time.Duration, now time.Time) {
	a, ok := bucket[key]
	if !ok || now.Sub(a.firstSeen) > window {
		a = &loginAttempt{firstSeen: now}
		bucket[key] = a
	}
	a.fails++
}

// sweepThrottle drops expired entries and bounds memory: without it, random
// IPs/usernames could grow the maps forever.
func sweepThrottle(now time.Time, window time.Duration) {
	for _, bucket := range []map[string]*loginAttempt{
		loginThrottle.byIP, loginThrottle.byAccount, loginThrottle.byRemote,
	} {
		for key, a := range bucket {
			if now.Sub(a.firstSeen) > window {
				delete(bucket, key)
			}
		}
	}
}

// remoteAddrKey returns the un-spoofable peer address of a request, with the
// ephemeral port stripped.
//
// c.IP() is attacker-controlled whenever server.trusted_proxies covers the
// peer's range: Fiber then takes it from X-Forwarded-For, so keying the throttle
// on it alone lets anyone reset their own attempt budget by rotating the
// header. The socket peer cannot be forged by a header, so it is what the
// byRemote bucket is keyed on.
func remoteAddrKey(c *fiber.Ctx) string {
	if c == nil || c.Context() == nil {
		return ""
	}
	addr := c.Context().RemoteAddr().String()
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// loginBlocked reports whether this address or account exhausted its attempts.
func loginBlocked(c *fiber.Ctx, ip, account string) bool {
	remote := remoteAddrKey(c)
	loginThrottle.Lock()
	defer loginThrottle.Unlock()

	now := time.Now()
	window := loginThrottleWindow()
	max := loginThrottleMaxAttempts()
	loginThrottle.ops++
	if loginThrottle.ops%1024 == 0 {
		sweepThrottle(now, window)
	}

	return bucketBlocked(loginThrottle.byIP, ip, max, window, now) ||
		bucketBlocked(loginThrottle.byRemote, remote, max, window, now) ||
		bucketBlocked(loginThrottle.byAccount, account, max, window, now)
}

// authBlocked is the IP-only variant for endpoints without an account name
// (registration, comments, password change).
func authBlocked(c *fiber.Ctx, ip string) bool {
	remote := remoteAddrKey(c)
	loginThrottle.Lock()
	defer loginThrottle.Unlock()

	now := time.Now()
	window := loginThrottleWindow()
	max := loginThrottleMaxAttempts()
	return bucketBlocked(loginThrottle.byIP, ip, max, window, now) ||
		bucketBlocked(loginThrottle.byRemote, remote, max, window, now)
}

func recordLoginFailure(c *fiber.Ctx, ip, account string) {
	remote := remoteAddrKey(c)
	loginThrottle.Lock()
	defer loginThrottle.Unlock()

	now := time.Now()
	window := loginThrottleWindow()
	bucketRecord(loginThrottle.byIP, ip, window, now)
	bucketRecord(loginThrottle.byRemote, remote, window, now)
	if account != "" {
		bucketRecord(loginThrottle.byAccount, account, window, now)
	}
	if len(loginThrottle.byIP)+len(loginThrottle.byAccount)+len(loginThrottle.byRemote) > 20000 {
		sweepThrottle(now, window)
	}
}

func resetLoginAttempts(c *fiber.Ctx, ip, account string) {
	loginThrottle.Lock()
	defer loginThrottle.Unlock()
	delete(loginThrottle.byIP, ip)
	if host := remoteAddrKey(c); host != "" {
		delete(loginThrottle.byRemote, host)
	}
	if account != "" {
		delete(loginThrottle.byAccount, account)
	}
}

// ResetLoginAttemptsForIP clears throttle state. Test hook (the counters are
// process-global); clears everything to isolate tests.
func ResetLoginAttemptsForIP(ip string) {
	loginThrottle.Lock()
	defer loginThrottle.Unlock()
	loginThrottle.byIP = make(map[string]*loginAttempt)
	loginThrottle.byAccount = make(map[string]*loginAttempt)
}

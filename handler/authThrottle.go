package handlers

import (
	"sync"
	"time"

	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
)

// loginAttempt tracks consecutive failures in one window.
type loginAttempt struct {
	fails     int
	firstSeen time.Time
}

// The throttle is process-local by design (see below). Two buckets: per IP
// and per account name, so neither rotating addresses nor targeting one
// account bypasses it.
var loginThrottle struct {
	sync.Mutex
	byIP      map[string]*loginAttempt
	byAccount map[string]*loginAttempt
	ops       int
}

func init() {
	loginThrottle.byIP = make(map[string]*loginAttempt)
	loginThrottle.byAccount = make(map[string]*loginAttempt)
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
	for _, bucket := range []map[string]*loginAttempt{loginThrottle.byIP, loginThrottle.byAccount} {
		for key, a := range bucket {
			if now.Sub(a.firstSeen) > window {
				delete(bucket, key)
			}
		}
	}
}

// loginBlocked reports whether this address or account exhausted its attempts.
func loginBlocked(ip, account string) bool {
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
		bucketBlocked(loginThrottle.byAccount, account, max, window, now)
}

// authBlocked is the IP-only variant for endpoints without an account name
// (registration, comments, password change).
func authBlocked(ip string) bool {
	loginThrottle.Lock()
	defer loginThrottle.Unlock()

	now := time.Now()
	window := loginThrottleWindow()
	return bucketBlocked(loginThrottle.byIP, ip, loginThrottleMaxAttempts(), window, now)
}

func recordLoginFailure(ip, account string) {
	loginThrottle.Lock()
	defer loginThrottle.Unlock()

	now := time.Now()
	window := loginThrottleWindow()
	bucketRecord(loginThrottle.byIP, ip, window, now)
	if account != "" {
		bucketRecord(loginThrottle.byAccount, account, window, now)
	}
	if len(loginThrottle.byIP)+len(loginThrottle.byAccount) > 20000 {
		sweepThrottle(now, window)
	}
}

func resetLoginAttempts(ip, account string) {
	loginThrottle.Lock()
	defer loginThrottle.Unlock()
	delete(loginThrottle.byIP, ip)
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

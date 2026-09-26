package handlers

import (
	"sync"
	"time"

	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
)

// loginAttempt tracks consecutive failures from one client address.
type loginAttempt struct {
	fails     int
	firstSeen time.Time
}

var loginAttempts struct {
	sync.Mutex
	byIP map[string]*loginAttempt
}

func init() {
	loginAttempts.byIP = make(map[string]*loginAttempt)
}

// dummyPasswordHash absorbs a bcrypt comparison when the username does not
// exist, so unknown-user and wrong-password logins take the same time.
var dummyPasswordHash []byte

func init() {
	hash, err := bcrypt.GenerateFromPassword([]byte("goxcms-dummy-password"), bcrypt.MinCost)
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

// loginBlocked reports whether this address exhausted its login attempts.
func loginBlocked(ip string) bool {
	loginAttempts.Lock()
	defer loginAttempts.Unlock()

	a, ok := loginAttempts.byIP[ip]
	if !ok {
		return false
	}
	if time.Since(a.firstSeen) > loginThrottleWindow() {
		delete(loginAttempts.byIP, ip)
		return false
	}
	return a.fails >= loginThrottleMaxAttempts()
}

func recordLoginFailure(ip string) {
	loginAttempts.Lock()
	defer loginAttempts.Unlock()

	a, ok := loginAttempts.byIP[ip]
	if !ok || time.Since(a.firstSeen) > loginThrottleWindow() {
		a = &loginAttempt{firstSeen: time.Now()}
		loginAttempts.byIP[ip] = a
	}
	a.fails++
}

func resetLoginAttempts(ip string) {
	loginAttempts.Lock()
	defer loginAttempts.Unlock()
	delete(loginAttempts.byIP, ip)
}

// ResetLoginAttemptsForIP clears throttle state for one address. Test hook
// (the counter is process-global and keyed by client IP).
func ResetLoginAttemptsForIP(ip string) {
	resetLoginAttempts(ip)
}

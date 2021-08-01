package server

import (
	"net"
	"strings"

	"github.com/pkg/errors"
)

type config struct {
	BaseURL       string `ini:"base_url"`
	BaseDir       string `ini:"base_dir"`
	StaticDir     string `ini:"static_dir"`
	TemplateDir   string `ini:"template_dir"`
	ListenAddress string `ini:"listen_address"`
	SecureCookie  bool   `ini:"secure_cookie"`
	CSRFKey       string `ini:"csrf_key"`
	OrgLogin      string `ini:"org_login"`
	OrgPassword   string `ini:"org_password"`
	SessionSecret string `ini:"session_secret"`
	SessionMaxAge int    `ini:"session_max_age"`
	SMSActive     bool   `ini:"sms_active"`
	SMSWhitelist  string `ini:"sms_whitelist"`
	// computed during initialization
	smsWhitelist []net.IP
}

func (c *config) init() error {
	if c.SMSWhitelist != "" {
		for _, address := range strings.Split(c.SMSWhitelist, ",") {
			ip := net.ParseIP(strings.TrimSpace(address))
			if ip == nil {
				return errors.Errorf("Cannot parse IP '%s' from sms_whitelist field", address)
			}
			c.smsWhitelist = append(c.smsWhitelist, ip)
		}
	}
	return nil
}

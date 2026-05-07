package utils

import (
	"strings"

	"github.com/gofiber/fiber/v3"
)

func GetFirstIP(ip string) string {
	if ip == "" || !strings.ContainsRune(ip, ',') {
		return ip
	}

	// first IP in list is the client's
	// get first non-empty IP
	ips := strings.Split(ip, ",")
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip != "" {
			return ip
		}
	}

	return ip // just return the original input if we couldn't parse it out for some reason
}

// GetIP from request
// If the request has a list of IPs, the first one is returned.
func GetIP(c fiber.Ctx) string {
	ip := fiber.Locals[string](c, "ip")
	if ip == "" {
		ip = GetFirstIP(c.IP())
		if ip == "" {
			ip = GetFirstIP(c.RequestCtx().RemoteIP().String())
		}
		c.Locals("ip", ip)
	}
	return ip
}

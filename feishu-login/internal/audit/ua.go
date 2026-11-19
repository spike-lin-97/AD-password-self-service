package audit

import "strings"

// ParseOS 从 User-Agent 解析操作系统名称。
func ParseOS(ua string) string {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return "—"
	}
	low := strings.ToLower(ua)
	switch {
	case strings.Contains(low, "iphone") || strings.Contains(low, "ipad") || strings.Contains(low, "ipod"):
		return "iOS"
	case strings.Contains(low, "android"):
		return "Android"
	case strings.Contains(low, "windows phone"):
		return "Windows Phone"
	case strings.Contains(low, "windows nt 10.0"):
		if strings.Contains(low, "windows 11") {
			return "Windows 11"
		}
		return "Windows 10/11"
	case strings.Contains(low, "windows nt 6.3"):
		return "Windows 8.1"
	case strings.Contains(low, "windows nt 6.2"):
		return "Windows 8"
	case strings.Contains(low, "windows nt 6.1"):
		return "Windows 7"
	case strings.Contains(low, "windows"):
		return "Windows"
	case strings.Contains(low, "cros"):
		return "ChromeOS"
	case strings.Contains(low, "macintosh") || strings.Contains(low, "mac os"):
		return "macOS"
	case strings.Contains(low, "harmonyos") || strings.Contains(low, "openharmony"):
		return "HarmonyOS"
	case strings.Contains(low, "ubuntu"):
		return "Ubuntu"
	case strings.Contains(low, "fedora"):
		return "Fedora"
	case strings.Contains(low, "linux"):
		return "Linux"
	default:
		return "未知"
	}
}

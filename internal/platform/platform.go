package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Platform is the OS object. Windows, macOS, and Linux are separate types.
type Platform interface {
	Name() string
	MaxArgBytes() int
	ExtraBins(names []string) []string
	ExtraDirs() []string
	Wrap(bin string, args []string) (string, []string)
	AugmentEnv(env []string) []string
}

// HostPaths is the per-machine location object the platforms consult.
type HostPaths struct {
	Home     string
	LocalApp string
	AppData  string
}

type platformBase struct {
	host HostPaths
	sep  string
}

func (p platformBase) join(elem ...string) string {
	parts := make([]string, 0, len(elem))
	for _, e := range elem {
		if e == "" {
			continue
		}
		parts = append(parts, strings.TrimRight(e, `/\`))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, p.sep)
}

func (p platformBase) homeBins(name string, windows bool) []string {
	if p.host.Home == "" {
		return nil
	}
	out := []string{
		p.join(p.host.Home, ".local", "bin", name),
		p.join(p.host.Home, "bin", name),
		p.join(p.host.Home, ".cargo", "bin", name),
		p.join(p.host.Home, ".grok", "bin", name),
	}
	if windows {
		out = append(out,
			p.join(p.host.Home, ".local", "bin", name+".exe"),
			p.join(p.host.Home, "bin", name+".exe"),
			p.join(p.host.Home, ".cargo", "bin", name+".exe"),
			p.join(p.host.Home, ".grok", "bin", name+".exe"),
		)
	}
	return out
}

func (p platformBase) homeDirs() []string {
	if p.host.Home == "" {
		return nil
	}
	return []string{
		p.join(p.host.Home, ".local", "bin"),
		p.join(p.host.Home, "bin"),
		p.join(p.host.Home, ".cargo", "bin"),
		p.join(p.host.Home, ".grok", "bin"),
	}
}

func LiveHostPaths() HostPaths {
	home, _ := os.UserHomeDir()
	return HostPaths{
		Home:     home,
		LocalApp: os.Getenv("LOCALAPPDATA"),
		AppData:  os.Getenv("APPDATA"),
	}
}

func NewPlatform(goos string, host HostPaths) Platform {
	switch goos {
	case "windows":
		return WindowsPlatform{platformBase{host: host, sep: `\`}}
	case "darwin":
		return DarwinPlatform{unixPlatform{platformBase: platformBase{host: host, sep: "/"}, name: "darwin"}}
	default:
		return LinuxPlatform{unixPlatform{platformBase: platformBase{host: host, sep: "/"}, name: "linux"}}
	}
}

func HostPlatform() Platform {
	return NewPlatform(runtime.GOOS, LiveHostPaths())
}

type unixPlatform struct {
	platformBase
	name string
}

func (u unixPlatform) Name() string { return u.name }

func (u unixPlatform) MaxArgBytes() int { return 100000 }

func (u unixPlatform) Wrap(bin string, args []string) (string, []string) {
	return bin, args
}

func unixAugment(p Platform, env []string) []string {
	return splicePath(env, "PATH", "PATH=", ":", p.ExtraDirs())
}

func (u unixPlatform) ExtraBins(names []string) []string {
	var out []string
	for _, name := range names {
		out = append(out, u.homeBins(name, false)...)
		out = append(out, u.join("/usr/local/bin", name))
		if u.host.Home != "" {
			out = append(out, u.join(u.host.Home, ".npm-global", "bin", name))
		}
	}
	return out
}

// DarwinPlatform is macOS, including Homebrew paths GUI apps often omit from PATH.
type DarwinPlatform struct{ unixPlatform }

func (d DarwinPlatform) AugmentEnv(env []string) []string { return unixAugment(d, env) }

func (d DarwinPlatform) ExtraDirs() []string {
	dirs := d.homeDirs()
	return append(dirs, "/opt/homebrew/bin", "/usr/local/bin")
}

func (d DarwinPlatform) ExtraBins(names []string) []string {
	var out []string
	for _, name := range names {
		out = append(out, d.homeBins(name, false)...)
		out = append(out, d.join("/opt/homebrew/bin", name), d.join("/usr/local/bin", name))
		if d.host.Home != "" {
			out = append(out, d.join(d.host.Home, ".npm-global", "bin", name))
		}
	}
	return out
}

// LinuxPlatform is Linux and other Unix.
type LinuxPlatform struct{ unixPlatform }

func (l LinuxPlatform) AugmentEnv(env []string) []string { return unixAugment(l, env) }

func (l LinuxPlatform) ExtraDirs() []string {
	return append(l.homeDirs(), "/usr/local/bin")
}

func (l LinuxPlatform) ExtraBins(names []string) []string {
	var out []string
	for _, name := range names {
		out = append(out, l.homeBins(name, false)...)
		out = append(out, l.join("/usr/local/bin", name), l.join("/usr/bin", name))
		if l.host.Home != "" {
			out = append(out, l.join(l.host.Home, ".npm-global", "bin", name))
		}
	}
	return out
}

// WindowsPlatform handles PATHEXT shims, npm .cmd files, and the 8191-char argv cap.
type WindowsPlatform struct{ platformBase }

func (w WindowsPlatform) Name() string { return "windows" }

func (w WindowsPlatform) MaxArgBytes() int { return 6000 }

func (w WindowsPlatform) ExtraDirs() []string {
	dirs := w.homeDirs()
	if w.host.LocalApp != "" {
		dirs = append(dirs, w.join(w.host.LocalApp, "npm"))
	}
	if w.host.AppData != "" {
		dirs = append(dirs, w.join(w.host.AppData, "npm"))
	}
	return dirs
}

func (w WindowsPlatform) ExtraBins(names []string) []string {
	var out []string
	for _, name := range names {
		out = append(out, w.homeBins(name, true)...)
		if w.host.LocalApp != "" {
			out = append(out,
				w.join(w.host.LocalApp, name, "bin", name+".exe"),
				w.join(w.host.LocalApp, name, "bin", name),
				w.join(w.host.LocalApp, "Programs", name, name+".exe"),
				w.join(w.host.LocalApp, "npm", name+".cmd"),
				w.join(w.host.LocalApp, "npm", name+".exe"),
			)
		}
		if w.host.AppData != "" {
			out = append(out,
				w.join(w.host.AppData, "npm", name+".cmd"),
				w.join(w.host.AppData, "npm", name+".exe"),
				w.join(w.host.AppData, "npm", name),
			)
		}
	}
	return out
}

func (w WindowsPlatform) Wrap(bin string, args []string) (string, []string) {
	ext := strings.ToLower(filepath.Ext(bin))
	if ext == ".cmd" || ext == ".bat" {
		return "cmd.exe", append([]string{"/c", bin}, args...)
	}
	return bin, args
}

func (w WindowsPlatform) AugmentEnv(env []string) []string {
	key, prefix := "PATH", "PATH="
	for _, e := range env {
		if len(e) >= 5 && strings.EqualFold(e[:5], "PATH=") {
			key = e[:4]
			prefix = e[:5]
			break
		}
	}
	return splicePath(env, key, prefix, ";", w.ExtraDirs())
}

func splicePath(env []string, key, prefix, listSep string, dirs []string) []string {
	if len(dirs) == 0 {
		return env
	}
	head := strings.Join(dirs, listSep)
	out := make([]string, 0, len(env)+1)
	found := false
	for _, e := range env {
		match := strings.HasPrefix(e, prefix)
		if !match && len(e) >= len(prefix) && strings.EqualFold(e[:len(prefix)], prefix) {
			match = true
			prefix = e[:len(prefix)]
			key = e[:len(prefix)-1]
		}
		if match {
			out = append(out, prefix+head+listSep+e[len(prefix):])
			found = true
			continue
		}
		out = append(out, e)
	}
	if !found {
		out = append(out, key+"="+head)
	}
	return out
}

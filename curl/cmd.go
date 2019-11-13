package curl

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/alecthomas/kingpin"
	"github.com/vearutop/plt/loadgen"
)

type flags struct {
	Header      []string
	HeaderMap   map[string]string
	URL         string
	Method      string
	Data        string
	Compressed  bool
	NoKeepalive bool
}

func AddCommand(lf *loadgen.Flags) {
	var (
		flags         flags
		captureString = map[string]*string{
			"url":     &flags.URL,
			"request": &flags.Method,
			"data":    &flags.Data,
		}
		captureBool = map[string]*bool{
			"compressed":   &flags.Compressed,
			"no-keepalive": &flags.NoKeepalive,
		}
	)

	curl := kingpin.Command("curl", "Repetitive HTTP transfer")

	curl.Flag("2.0", `Workaround of Firefox "Copy as cURL" incompatibility.`).Bool()
	curl.Arg("url", "The URL.").StringVar(&flags.URL)

	reg := regexp.MustCompile(`(?P<Short>-[\w],)?\s--(?P<Long>[\w\-.]+)(?P<Arg>\s[^\s]+)?\s+(?P<Desc>.+)$`)
	for _, line := range strings.Split(curlHelp, "\n") {
		m := reg.FindStringSubmatch(line)
		if len(m) == 0 {
			panic("Impossibru: " + line)
		}

		short := strings.Trim(m[1], "-,")
		long := m[2]
		arg := strings.Trim(m[3], " ")
		desc := strings.Trim(m[4], "")

		// help is already defined by kingpin.
		if long == "help" {
			continue
		}

		if long != "header" && captureString[long] == nil && captureBool[long] == nil {
			desc = desc + " (flag ignored)"
		}

		f := curl.Flag(long, desc+".")

		if arg != "" {
			if long == "header" {
				f.StringsVar(&flags.Header)
			} else {
				if s, ok := captureString[long]; ok {
					f.StringVar(s)
				} else {
					f.String()
				}
			}

			f.PlaceHolder(arg)
		} else {
			if b, ok := captureBool[long]; ok {
				f.BoolVar(b)
			} else {
				f.Bool()
			}
		}

		if short != "" {
			f.Short(rune(short[0]))
		}
	}

	curl.Action(func(kp *kingpin.ParseContext) error {
		flags.HeaderMap = make(map[string]string, len(flags.Header))
		for _, h := range flags.Header {
			parts := strings.SplitN(h, ":", 2)
			if len(parts) != 2 {
				continue
			}
			flags.HeaderMap[http.CanonicalHeaderKey(parts[0])] = strings.Trim(parts[1], " ")
		}
		if flags.Compressed {
			if _, ok := flags.HeaderMap["Accept-Encoding"]; !ok {
				flags.HeaderMap["Accept-Encoding"] = "gzip, deflate"
			}
		}
		run(lf, flags)
		return nil
	})
}

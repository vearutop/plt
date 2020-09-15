// Package curl implements curl command line interface.
package curl

import (
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/alecthomas/kingpin"
	"github.com/vearutop/plt/fasthttp"
	"github.com/vearutop/plt/loadgen"
	"github.com/vearutop/plt/nethttp"
)

// AddCommand registers curl command into CLI app.
func AddCommand(lf *loadgen.Flags) {
	var (
		flags   nethttp.Flags
		capture struct {
			header     []string
			data       []string
			compressed bool
			user       string
		}
		captureStrings = map[string]*[]string{
			"header":         &capture.header,
			"data":           &capture.data,
			"data-raw":       &capture.data,
			"data-ascii":     &capture.data,
			"data-binary":    &capture.data,
			"data-urlencode": &capture.data,
		}
		captureString = map[string]*string{
			"url":     &flags.URL,
			"request": &flags.Method,
			"user":    &capture.user,
		}
		captureBool = map[string]*bool{
			"compressed":   &capture.compressed,
			"no-keepalive": &flags.NoKeepalive,
		}
		ignoredString = map[string]*string{}
		ignoredBool   = map[string]*bool{}
	)

	curl := kingpin.Command("curl", "Repetitive HTTP transfer")

	curl.Flag("fast", "Use fasthttp to achieve higher request rate").BoolVar(&flags.Fast)
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
		if long == "help" || long == "version" {
			continue
		}

		if long != "header" && captureString[long] == nil && captureBool[long] == nil {
			desc += " (flag ignored)"
		}

		f := curl.Flag(long, desc+".")

		emptyArg := func() {
			if b, ok := captureBool[long]; ok {
				f.BoolVar(b)
			} else {
				ignoredBool[long] = f.Bool()
			}
		}

		nonEmptyArg := func() {
			if ss, ok := captureStrings[long]; ok {
				f.StringsVar(ss)
			} else {
				if s, ok := captureString[long]; ok {
					f.StringVar(s)
				} else {
					ignoredString[long] = f.String()
				}
			}

			f.PlaceHolder(arg)
		}

		if arg != "" {
			nonEmptyArg()
		} else {
			emptyArg()
		}

		if short != "" {
			f.Short(rune(short[0]))
		}
	}

	curl.Action(func(kp *kingpin.ParseContext) error {
		ignoredFlags := make([]string, 0)
		for f, v := range ignoredString {
			if v != nil && *v != "" {
				ignoredFlags = append(ignoredFlags, f)
			}
		}
		for f, v := range ignoredBool {
			if v != nil && *v {
				ignoredFlags = append(ignoredFlags, f)
			}
		}
		if len(ignoredFlags) > 0 {
			log.Printf("Warning, these Flags are ignored: %v\n", ignoredFlags)
		}

		if len(capture.data) == 1 {
			flags.Body = capture.data[0]
		} else if len(capture.data) > 1 {
			flags.Body = strings.Join(capture.data, "&")
		}

		flags.HeaderMap = make(map[string]string, len(capture.header))
		if capture.user != "" {
			if !strings.Contains(capture.user, ":") {
				return errors.New("user parameter must be in form user:pass")
			}
			flags.HeaderMap["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(capture.user))
		}

		if flags.Body != "" {
			flags.HeaderMap["Content-Type"] = "application/x-www-form-urlencoded"

			if flags.Method == "" {
				flags.Method = http.MethodPost
			}
		}

		for _, h := range capture.header {
			parts := strings.SplitN(h, ":", 2)
			if len(parts) != 2 {
				continue
			}
			flags.HeaderMap[http.CanonicalHeaderKey(parts[0])] = strings.Trim(parts[1], " ")
		}
		if capture.compressed {
			if _, ok := flags.HeaderMap["Accept-Encoding"]; !ok {
				flags.HeaderMap["Accept-Encoding"] = "gzip, deflate"
			}
		}
		if !strings.HasPrefix(strings.ToLower(flags.URL), "http://") &&
			!strings.HasPrefix(strings.ToLower(flags.URL), "https://") {
			flags.URL = "http://" + flags.URL
		}
		run(*lf, flags)
		return nil
	})
}

func run(lf loadgen.Flags, f nethttp.Flags) {
	lf.Prepare()

	if f.Fast {
		j := fasthttp.NewJobProducer(f)
		loadgen.Run(lf, j)
		j.Print()
	} else {
		j := nethttp.NewJobProducer(f, lf)
		loadgen.Run(lf, j)
		j.Print()
	}
}

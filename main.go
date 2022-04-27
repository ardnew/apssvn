package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/ardnew/apssvn/cache"
)

var (
	PROJECT   string
	IMPORT    string
	VERSION   string
	BUILDTIME string
	PLATFORM  string
	BRANCH    string
	REVISION  string
)

const (
	cacheName   = ".apsrepo"
	hostName    = "rstok3-dev02"
	svnAddrPort = "http://" + hostName + ":3690"
	webURLRoot  = "viewvc"
	svnURLRoot  = "svn"
)

const newline = "\r\n"

func exeName() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Base(exe)
	}
	return filepath.Base(os.Args[0])
}

func usage(set *flag.FlagSet) {
	ww := &wordWrap{column: 80, indent: "  ", indentFirst: true}
	fmt.Printf("%s %s %s %s@%s %s"+newline,
		IMPORT, VERSION, PLATFORM, BRANCH, REVISION, BUILDTIME)
	fmt.Println("USAGE")
	fmt.Printf(ww.wrap(exeName(), "[flags] [repo-pattern ...] [-- svn-command-line ...]"))
	fmt.Println()
	fmt.Println("FLAGS (mnemonics shown in [brackets])")
	// Determine the maximum width of the left-hand side containing "-x foo" among
	// all defined flags
	margin := 0
	set.VisitAll(func(f *flag.Flag) {
		name, _ := flag.UnquoteUsage(f)
		if width := len(name) + len(f.Name); width > margin {
			margin = width
		}
	})
	ww.indentFirst = false
	ww.indent = fmt.Sprintf("  %*s ", margin+4, "")
	set.VisitAll(func(f *flag.Flag) {
		name, desc := flag.UnquoteUsage(f)
		flagName := fmt.Sprintf("-%s %s", f.Name, name)
		ww.caption = fmt.Sprintf("  %-*s ", margin+4, flagName)
		fmt.Printf(ww.wrap(desc))
	})
	ww.indentFirst = true
	ww.indent = "  "
	ww.caption = ""
	fmt.Println()
	fmt.Println("NOTES")
	fmt.Printf(ww.wrap("All arguments following the first occurrence of \"--\" are",
		"forwarded (in the same order they were given) to each \"svn\" command",
		"generated. Since the same command may run with multiple repositories,",
		"placeholder variables may be used in the given command line, which are then",
		"substituted with attributes from the target repository each time the \"svn\"",
		"command is run."))
	fmt.Println()
	ww.indent = "      "
	fmt.Printf(ww.wrap("@ = repository URL (must be first character in word)"))
	fmt.Printf(ww.wrap("^ = repository base name"))
	fmt.Printf(ww.wrap("& = preceding URL/path argument"))
	fmt.Printf(ww.wrap("$ = last path component (basename) of \"&\""))
	fmt.Printf(ww.wrap("! = parent path component (basename of dirname) of \"&\""))
	ww.indent = "  "
	fmt.Println()
	fmt.Printf(ww.wrap("For example, exporting a common tag from all repositories",
		"with \"DAPA\" in the name into respectively-named subdirectories of the",
		"current directory:"))
	fmt.Println()
	ww.indent = "      "
	fmt.Printf(ww.wrap("%%", exeName(), "DAPA", "--", "export @/tags/foo ./^/tags/foo"))
	ww.indent = "  "
	fmt.Println()
}

func main() {

	repoCache := cache.New(cacheName)

	defBaseURL := svnAddrPort

	//argBrowse := flag.Bool("b", false, "open Web URL with Web browser")
	argCaseSen := flag.Bool("c", false, "use [case]-sensitive matching")
	argDryRun := flag.Bool("d", false, "print commands which would be executed ([dry-run])")
	argRepoFile := flag.String("f", repoCache.FilePath, "use repository definitions from [file] `path`")
	argLogin := flag.String("l", "", "use `user:pass` to authenticate with SVN or REST API ([login])")
	argMatchAny := flag.Bool("o", false, "use logical-[or] matching if multiple patterns given")
	argQuiet := flag.Bool("q", false, "suppress all non-essential and error messages ([quiet])")
	argBaseURL := flag.String("s", defBaseURL, "prepend [server] `url` to all constructed URLs")
	argUpdate := flag.Bool("u", false, "[update] cached repository definitions from server")
	argWebURL := flag.Bool("w", false, "construct [web] URLs instead of repository URLs")
	flag.Usage = func() { usage(flag.CommandLine) }

	flag.Parse()

	if *argQuiet {
		log.SetOutput(io.Discard)
	} else {
		log.SetFlags(log.LstdFlags | log.Lmsgprefix)
		log.SetPrefix("-- ")
	}

	// Keep all arguments other than the first occurrence of "--".
	patArg := []string{}
	cmdArg := []string{}
	ptrArg := &patArg
	for _, a := range flag.Args() {
		if strings.TrimSpace(a) == "--" {
			ptrArg = &cmdArg
		} else {
			*ptrArg = append(*ptrArg, a)
		}
	}

	user, pass := "", ""
	if *argLogin != "" {
		s := strings.SplitN(*argLogin, ":", 2)
		if len(s) > 0 {
			user = s[0]
		}
		if len(s) > 1 {
			pass = s[1]
		}
	}

	err := repoCache.Sync(*argRepoFile, *argUpdate, user, pass)
	if nil != err {
		log.Fatalln(err)
	}

	*argBaseURL = strings.TrimRight(*argBaseURL, "/")

	urlRoot := svnURLRoot
	if *argWebURL {
		urlRoot = webURLRoot
	}

	listMatch := func(match []string) {
		for _, repo := range match {
			fmt.Printf("%s/%s/%s", *argBaseURL, urlRoot, repo)
			fmt.Println()
		}
	}

	runMatch := func(match []string) {
		for _, repo := range match {
			url := fmt.Sprintf("%s/%s/%s", *argBaseURL, urlRoot, repo)

			expArg := make([]string, len(cmdArg))
			for i, s := range cmdArg {
				prec := ""
				if i > 0 {
					prec = expArg[i-1]
				}
				expArg[i] = expand(s, url, repo, prec)
			}
			// Print the command line being executed
			var cli strings.Builder
			for i, s := range expArg {
				if i > 0 {
					cli.WriteRune(' ')
				}
				if strings.ContainsAny(s, " \t\n$&|<>;`~#{}[]*?!") {
					cli.WriteString("'" + s + "'")
				} else {
					cli.WriteString(s)
				}
			}
			log.Println("| svn " + cli.String())
			if !*argDryRun {
				out, err := run(expArg...)
				if out != nil && out.Len() > 0 {
					fmt.Print(out.String())
				}
				switch {
				case errors.Is(err, &exec.ExitError{}):
					log.Fatalln("error:", string(err.(*exec.ExitError).Stderr))
				case err != nil:
					log.Fatalln("error:", err.Error())
				}
			}
		}
	}

	if len(patArg) == 0 {
		if len(cmdArg) == 0 {
			// no arguments or patterns given, print all known repositories
			for _, repo := range repoCache.List {
				fmt.Printf("%s/%s/%s", *argBaseURL, urlRoot, repo)
				fmt.Println()
			}
		} else {
		}
	} else {
		if *argMatchAny {
			for _, arg := range patArg {
				match, err := repoCache.Match([]string{arg}, !*argCaseSen)
				if nil != err {
					log.Println("warning: skipping invalid expression:", arg)
				}
				if len(cmdArg) == 0 {
					listMatch(match)
				} else {
					runMatch(match)
				}
			}
		} else {
			match, err := repoCache.Match(patArg, !*argCaseSen)
			if nil != err {
				log.Fatalln("error: invalid expression(s):",
					"[", strings.Join(patArg, ", "), "]")
			}
			if len(match) == 0 {
				log.Fatalln("error: no repository found matching expression(s):",
					"[", strings.Join(patArg, ", "), "]")
			}
			if len(cmdArg) == 0 {
				listMatch(match)
			} else {
				runMatch(match)
			}
		}
	}
}

func expand(str string, url, base, prec string) string {
	for len(str) > 0 && str[0] == '@' {
		str = url + str[1:]
	}
	bn := filepath.Base(prec)
	pn := filepath.Base(filepath.Dir(prec))
	fmt.Printf("url=%q, base=%q, prec=%q, bn=%q, pn=%q\n", url, base, prec, bn, pn)
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(
		strings.ReplaceAll(str, "^", base), "&", prec), "$", bn), "!", pn)
}

func nonEmpty(arg ...string) []string {
	result := make([]string, 0, len(arg))
	for _, s := range arg {
		if t := strings.TrimSpace(s); t != "" {
			result = append(result, s) // keep the untrimmed original
		}
	}
	return result
}

func run(arg ...string) (*strings.Builder, error) {
	var b, e strings.Builder
	cmd := exec.Command("svn", nonEmpty(arg...)...)
	cmd.Stdout = &b
	cmd.Stderr = &e
	err := cmd.Run()
	if e.Len() > 0 {
		if err != nil {
			return &b, fmt.Errorf("%w\r\n%s", err, strings.TrimSpace(e.String()))
		}
		return &b, errors.New(strings.TrimSpace(e.String()))
	}
	return &b, err
}

type wordWrap struct {
	column      int
	indent      string
	caption     string
	indentFirst bool
}

func (ww *wordWrap) wrap(word ...string) string {
	var sb strings.Builder
	var rp []rune
	for i, w := range word {
		if len(w) > 0 {
			// No visible symbols exist after this word
			last := (i+1 == len(word)) ||
				(strings.TrimSpace(strings.Join(word[i+1:], "")) == "")
			// Word is non-empty
			if t := strings.TrimSpace(w); t != "" {
				// Word contains a visible symbol
				rw, rt := []rune(w), []rune(t)
				// Word is the first word being added
				first := sb.Len() == 0
				// Word is a punctuation character
				punct := (len(rt) == 1) && unicode.IsPunct(rt[0])
				// Word begins with whitespace
				wsBeg := unicode.IsSpace(rw[0])
				// Previous word ends with whitespace
				wsEnd := (len(rp) > 0) && unicode.IsSpace(rp[len(rp)-1])
				if !first && !punct && !wsBeg && !wsEnd {
					sb.WriteRune(' ')
				}
				if punct {
					sb.WriteString(t)
				} else {
					if last {
						sb.WriteString(w[:strings.LastIndex(w, t)+len(t)])
					} else {
						sb.WriteString(w)
					}
				}
				rp = rw
			}
			if last {
				break
			}
		}
	}
	var lb strings.Builder
	var ls string
	var ln int
	if ww.indentFirst {
		ls = ww.indent
	} else {
		if ww.caption != "" {
			ls = ww.caption
		} else {
			for _, c := range sb.String() {
				if !unicode.IsSpace(c) {
					break
				}
				ls += string(c)
			}
		}
	}
	lf := strings.Fields(sb.String())
	for i, w := range lf {
		if len(ls)+len(w) >= ww.column {
			lb.WriteString(ls + newline)
			ls = ww.indent
			ln++
			if len(ww.indent)+len(w) >= ww.column {
				ml := ww.column - len(ww.indent) - 1
				for j := 0; j < len(w); j += ml {
					if ml > len(w[j:]) {
						ls += w[j:]
						break
					}
					lb.WriteString(ww.indent + w[j:ml] + "-" + newline)
					ln++
				}
				break
			}
		} else {
			if i > 0 {
				ls += " "
			}
		}
		ls += w
	}
	if ls != ww.indent {
		lb.WriteString(ls + newline)
	}
	return strings.TrimRight(lb.String(), "\r\n") + newline
}

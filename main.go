package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
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
	repoFileName   = ".apsrepo"
	serverAddrPort = "http://rstok3-dev02:3690"
	webURLRoot     = "viewvc"
	svnURLRoot     = "svn"
)

const newline = "\r\n"

func exeName() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Base(exe)
	}
	return filepath.Base(os.Args[0])
}

func usage(set *flag.FlagSet) {
	fmt.Println(ln(fmt.Sprintf("%s %s %s %s@%s %s",
		IMPORT, VERSION, PLATFORM, BRANCH, REVISION, BUILDTIME)))
	fmt.Println("USAGE")
	fmt.Println(ln(fmt.Sprintf("  %s [flags] [filter-regexp ...] [-- svn-command ...] [+path]",
		exeName())))
	fmt.Println("FLAGS")
	set.PrintDefaults()
	fmt.Println()
	fmt.Println("NOTES")
	fmt.Printf(ln("  The \"flag\" package in Go's standard library requires all option flags be"))
	fmt.Printf(ln("  specified preceding all non-flag arguments. The non-flag arguments of %s"), exeName())
	fmt.Printf(ln("  however are used to express svn subcommands, which often accept file paths as"))
	fmt.Printf(ln("  their trailing arguments. This conflict results in an awkward %s command-line"), exeName())
	fmt.Printf(ln("  syntax where the target path of svn subcommands ('-p' flag) must be expressed"))
	fmt.Printf(ln("  near the front of the command-line, and the svn subcommand is expressed at the"))
	fmt.Printf(ln("  end. For example:"))
	fmt.Println()
	fmt.Printf(ln("    %s -p branches/b ^foo -- ls -v       // svn ls -v <REPO>/branches/b"), exeName())
	fmt.Println()
	fmt.Printf(ln("  Thus, to change the path given to \"svn ls\", the user must navigate to the front"))
	fmt.Printf(ln("  of the command-line and edit the argument given to flag '-p'. To address this"))
	fmt.Printf(ln("  frustration, an alternative syntax may be provided. Ignore the '-p' flag entirely."))
	fmt.Printf(ln("  Instead, anywhere in the svn subcommand (that is, anywhere following the"))
	fmt.Printf(ln("  end-of-options delimiter '--'), you may specify a file path by prefixing it with a"))
	fmt.Printf(ln("  single '+'. This allows the user to place the path at the end of the command-line"))
	fmt.Printf(ln("  for convenient repeated editing, or anywhere within the svn subcommand that feels"))
	fmt.Printf(ln("  most natural. The example above could instead be expressed as:"))
	fmt.Println()
	fmt.Printf(ln("    %s ^foo -- ls -v +branches/b"), exeName())
	fmt.Println()
}

func main() {

	defRepoPath := repoFilePath(repoFileName)
	defBaseURL := serverAddrPort

	argMatchAny := flag.Bool("a", false, "select repositories matching any given individual argument")
	//argBrowse := flag.Bool("b", false, "open Web URL with Web browser")
	argCaseSen := flag.Bool("c", false, "use case-sensitive matching")
	argDryRun := flag.Bool("d", false, "do nothing but print commands which would be executed (dry-run)")
	argRepoFile := flag.String("f", defRepoPath, "use repository definitions from `file`")
	argOutPath := flag.String("o", "", "command output `path` (variables: @=repo, ^=relpath, %=repo/relpath)")
	argRelPath := flag.String("p", "", "append `path` to all constructed URLs (see NOTES for alternative)")
	argQuiet := flag.Bool("q", false, "suppress all non-essential and error messages (quiet)")
	argBaseURL := flag.String("s", defBaseURL, "prepend `server` to all constructed URLs")
	argWebURL := flag.Bool("w", false, "construct Web URLs instead of repository URLs")
	flag.Usage = func() { usage(flag.CommandLine) }

	flag.Parse()

	if *argQuiet {
		log.SetOutput(io.Discard)
	}

	var expLen, cmdPos int
	for i, a := range flag.Args() {
		if a == "--" {
			if pos := i + 1; flag.NArg() > pos {
				cmdPos = pos
			}
			break
		}
		expLen++
	}

	var expArg, cmdArg []string

	if expLen > 0 {
		expArg = flag.Args()[:expLen]
	}
	if cmdPos > 0 {
		cmdArg = flag.Args()[cmdPos:]
	}

	// If user provides a path with "+" prefix anywhere in the command arguments,
	// use it as the argRelPath flag value. This makes path specification much
	// more convenient, without having to traverse back to the beginning of the
	// command line to change paths.
	for i, s := range cmdArg {
		if s[0] == '+' && len(s) > 1 {
			*argRelPath = s[1:]
			if len(cmdArg) > i+1 {
				copy(cmdArg[i:], cmdArg[i+1:])
			}
			cmdArg = cmdArg[:len(cmdArg)-1]
			break
		}
	}

	list, err := NewRepoList(*argRepoFile)
	if nil != err {
		log.Fatalln(err)
	}

	*argBaseURL = strings.TrimRight(*argBaseURL, "/")
	*argRelPath = strings.TrimLeft(*argRelPath, "/")

	urlRoot := svnURLRoot
	if *argWebURL {
		urlRoot = webURLRoot
	}

	procMatch := func(match []string) {
		if cmdPos == 0 {
			for _, rep := range match {
				if *argRelPath != "" {
					rep = fmt.Sprintf("%s/%s", rep, *argRelPath)
				}
				fmt.Printf("%s/%s/%s", *argBaseURL, urlRoot, rep)
				fmt.Println()
			}
		} else {
			for _, rep := range match {
				var outPath string
				base := rep
				if *argRelPath != "" {
					rep = fmt.Sprintf("%s/%s", rep, *argRelPath)
				}
				repo := fmt.Sprintf("%s/%s/%s", *argBaseURL, urlRoot, rep)
				if len(match) > 0 {
					if *argOutPath != "" {
						outPath = outputPath(*argOutPath, base, *argRelPath)
					}
					// Print the command line being executed
					var sb strings.Builder
					sb.WriteString("| svn")
					for _, s := range cmdArg {
						sb.WriteRune(' ')
						sb.WriteString(s)
					}
					sb.WriteRune(' ')
					sb.WriteString(repo)
					if outPath != "" {
						sb.WriteRune(' ')
						sb.WriteString(outPath)
					}
					log.Println(sb.String())
				}
				if !*argDryRun {
					out, err := run(repo, outPath, cmdArg...)
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
	}

	if expLen == 0 {
		// no arguments given, print all known repositories
		for _, rep := range *list {
			if *argRelPath != "" {
				rep = Repo(fmt.Sprintf("%s/%s", rep, *argRelPath))
			}
			fmt.Printf("%s/%s/%s", *argBaseURL, urlRoot, rep)
			fmt.Println()
		}
	} else {
		if *argMatchAny {
			for _, arg := range expArg {
				match, err := list.matches([]string{arg}, *argCaseSen)
				if nil != err {
					log.Println("warning: skipping invalid expression:", arg)
				}
				procMatch(match)
			}
		} else {
			match, err := list.matches(expArg, *argCaseSen)
			if nil != err {
				log.Fatalln("error: invalid expression(s):",
					"[", strings.Join(expArg, ", "), "]")
			}
			if len(match) == 0 {
				log.Fatalln("error: no repository found matching expression(s):",
					"[", strings.Join(expArg, ", "), "]")
			}
			procMatch(match)
		}
	}
}

func outputPath(pattern, repo, relPath string) string {
	const maxSubs = 100 // surely you can't be serious.
	sub := map[rune]string{
		// disallow keywords from infinitely substituting itself. this doesn't
		// prevent mutually-infinite recursion. don't call me shirley.
		'@': strings.ReplaceAll(repo, "@", "\\@"),
		'^': strings.ReplaceAll(relPath, "^", "\\^"),
		'%': strings.ReplaceAll(filepath.Join(repo, relPath), "%", "\\%"),
	}
	expand := func(s string) (string, bool) {
		didExpand := false
		for k, v := range sub {
			b := strings.Builder{}
			e := []rune(s)
			for i, r := range e {
				if r == k && (i < 1 || e[i-1] != '\\') {
					b.WriteString(v)
					didExpand = true
				} else {
					b.WriteRune(r)
				}
			}
			s = b.String()
		}
		return s, didExpand
	}
	for i := 0; i < maxSubs; i++ {
		s, didExpand := expand(pattern)
		if !didExpand {
			break
		}
		pattern = s
	}
	for k := range sub {
		pattern = strings.ReplaceAll(pattern, "\\"+string(k), string(k))
	}
	return pattern
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

func run(repo, out string, arg ...string) (*strings.Builder, error) {
	if out != "" {
		if err := os.MkdirAll(out, 0755); err != nil {
			return nil, err
		}
	}
	var b, e strings.Builder
	cmd := exec.Command("svn", append(arg, nonEmpty(repo, out)...)...)
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

func repoFilePath(name string) string {
	exists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}
	if home, err := os.UserHomeDir(); nil == err {
		if path := filepath.Join(home, name); exists(path) {
			return path
		}
	}
	if home, ok := os.LookupEnv("HOME"); ok {
		if path := filepath.Join(home, name); exists(path) {
			return path
		}
	}
	if exe, err := os.Executable(); nil == err {
		if path := filepath.Join(filepath.Dir(exe), name); exists(path) {
			return path
		}
	}
	if pwd, err := os.Getwd(); nil == err {
		if path := filepath.Join(pwd, name); exists(path) {
			return path
		}
	}
	return filepath.Join(".", name)
}

type Repo string
type RepoList []Repo

func NewRepoList(filePath string) (*RepoList, error) {

	file, err := os.Open(filePath)
	if nil != err {
		return nil, err
	}
	defer file.Close()

	list := RepoList{}
	scan := bufio.NewScanner(file)

	for scan.Scan() {
		list = append(list, Repo(scan.Text()))
	}

	if err := scan.Err(); nil != err {
		return nil, err
	}

	return &list, nil
}

func (l *RepoList) matches(pat []string, sen bool) ([]string, error) {

	exp := make([]*regexp.Regexp, len(pat))
	for i, p := range pat {
		if !sen {
			p = "(?i)" + p
		}
		e, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		exp[i] = e
	}

	m := []string{}
	for _, rep := range *l {
		match := false
		for _, e := range exp {
			if match = e.MatchString(string(rep)); !match {
				break
			}
		}
		if match {
			m = append(m, string(rep))
		}
	}

	return m, nil
}

func ln(word ...string) string {
	var sb strings.Builder
	var rp []rune
	var pp bool
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
				if pp = punct; pp {
					sb.WriteString(t)
				} else {
					if last {
						// Trim trailing whitespace from last word
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
	return strings.TrimRight(sb.String(), "\r\n") + newline
}

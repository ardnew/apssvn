package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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

func usage(set *flag.FlagSet) {
	fmt.Printf("%s version %s (%s@%s %s) %s\n",
		IMPORT, VERSION, BRANCH, REVISION, PLATFORM, BUILDTIME)
	set.PrintDefaults()
}

func main() {

	defRepoPath := repoFilePath(repoFileName)
	defBaseURL := serverAddrPort

	//argBrowse := flag.Bool("b", false, "open Web URL with Web browser")
	argCaseSen := flag.Bool("c", false, "use case-sensitive matching")
	argDryRun := flag.Bool("d", false, "print commands which would be executed (dry-run)")
	argRepoFile := flag.String("f", defRepoPath, "use repository definitions from `file`")
	argMatchAny := flag.Bool("o", false, "use logical-OR matching with all given arguments (default \"logical-AND\")")
	argRelPath := flag.String("p", "", "append `path` to all constructed URLs")
	argBaseURL := flag.String("s", defBaseURL, "prepend `server` to all constructed URLs")
	argWebURL := flag.Bool("w", false, "construct Web URLs instead of repository URLs")
	flag.Usage = func() { usage(flag.CommandLine) }

	flag.Parse()

	expArg := []string{}
	cmdArg := []string{}

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
	if expLen > 0 {
		expArg = flag.Args()[:expLen]
	}
	if cmdPos > 0 {
		cmdArg = flag.Args()[cmdPos:]
	}

	list, err := NewRepoList(*argRepoFile)
	if nil != err {
		log.Fatalln(err)
	}

	*argBaseURL = strings.TrimRight(*argBaseURL, "/")
	*argRelPath = strings.TrimLeft(*argRelPath, "/")

	rootURL := svnURLRoot
	if *argWebURL {
		rootURL = webURLRoot
	}

	procMatch := func(match []string) {
		if cmdPos == 0 {
			for _, rep := range match {
				if *argRelPath != "" {
					rep = fmt.Sprintf("%s/%s", rep, *argRelPath)
				}
				fmt.Printf("%s/%s/%s", *argBaseURL, rootURL, rep)
				fmt.Println()
			}
		} else {
			for _, rep := range match {
				if *argRelPath != "" {
					rep = fmt.Sprintf("%s/%s", rep, *argRelPath)
				}
				repo := fmt.Sprintf("%s/%s/%s", *argBaseURL, rootURL, rep)
				if len(match) > 0 {
					// Print the command line being executed
					var sb strings.Builder
					sb.WriteString("| svn")
					for _, s := range cmdArg {
						sb.WriteRune(' ')
						sb.WriteString(s)
					}
					sb.WriteRune(' ')
					sb.WriteString(repo)
					log.Println(sb.String())
				}
				if !*argDryRun {
					out, err := run(repo, cmdArg...)
					if out.Len() > 0 {
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
			fmt.Printf("%s/%s/%s", *argBaseURL, rootURL, rep)
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

func run(repo string, arg ...string) (*strings.Builder, error) {
	var b, e strings.Builder
	cmd := exec.Command("svn", append(arg, repo)...)
	cmd.Stdout = &b
	cmd.Stderr = &e
	err := cmd.Run()
	if e.Len() > 0 {
		if err != nil {
			err = fmt.Errorf("%w\r\n%s", err, strings.TrimSpace(e.String()))
		} else {
			err = errors.New(strings.TrimSpace(e.String()))
		}
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

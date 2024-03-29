package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

var (
	pHelp     = flag.Bool("h", false, "help")
	pRepo     = flag.String("repo", "", "repository url to be cloned")
	pBranch   = flag.String("branch", "", "branch to be cloned")
	pFromHash = flag.String("from", "", "from commit hash")
	pToHash   = flag.String("to", "", "to commit hash")
	pUser     = flag.String("user", "", "git user")
	pPassword = flag.String("password", "", "git password")
	pToken    = flag.String("token", "", "git token")
	pExplain  = flag.Bool("explain", false, "explain why a package is needed")
)

func parseFromHash(repo *git.Repository, hash string) (*object.Tree, map[string]string) {
	commit, _ := repo.CommitObject(plumbing.NewHash(hash))
	if commit == nil {
		iter, err := repo.CommitObjects()
		die(err)

		for {
			var err error
			commit, err = iter.Next()
			die(err)
			if strings.HasPrefix(commit.Hash.String(), hash) {
				break
			}
		}
	}
	if commit == nil {
		die(fmt.Errorf("commit %s not found", hash))
	}

	tree, err := commit.Tree()
	die(err)

	goSumFile, err := tree.File("go.sum")
	die(err)

	goSumContent, err := goSumFile.Contents()
	die(err)

	goSum := map[string]string{}
	for _, line := range strings.Split(goSumContent, "\n") {
		s := strings.Split(line, " ")
		if len(s) != 3 {
			continue
		}
		goSum[s[0]] = s[1]
	}

	return tree, goSum
}

func main() {
	flag.Parse()
	if *pHelp {
		println("go-diff -repo github.com/user/repo -branch master -from <HASH> -to <HASH>")
		os.Exit(0)
	}
	repoURL := *pRepo
	branch := *pBranch
	fromHash := *pFromHash
	toHash := *pToHash

	var auth transport.AuthMethod
	if tok := *pToken; tok != "" {
		auth = &http.TokenAuth{Token: tok}
	} else {
		user := *pUser
		pwd := *pPassword
		if user != "" && pwd != "" {
			auth = &http.BasicAuth{
				Username: user,
				Password: pwd,
			}
		}
	}

	var repo *git.Repository
	var err error

	if strings.HasPrefix(repoURL, "file://") {
		repo, err = git.PlainOpen(repoURL[7:])
	} else {
		repo, err = git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
			URL:           repoURL,
			ReferenceName: plumbing.NewBranchReferenceName(branch),
			Auth:          auth,
		})
	}
	die(err)

	fromTree, fromGoSum := parseFromHash(repo, fromHash)
	toTree, toGoSum := parseFromHash(repo, toHash)

	changes, err := toTree.Diff(fromTree)
	die(err)

	patch, err := changes.Patch()
	die(err)

	patches := patch.FilePatches()

	rawChanges := map[string]bool{}

	for _, file := range patches {
		from, to := file.Files()
		if from != nil && to != nil {
			rawChanges[from.Path()] = true
		} else if from != nil {
			rawChanges[from.Path()] = true
		}
	}

	goModFile, err := toTree.File("go.mod")
	die(err)

	goMod, err := goModFile.Contents()
	die(err)

	packageName := ""
	for _, line := range strings.Split(goMod, "\n") {
		if strings.HasPrefix(line, "module ") {
			packageName = strings.Split(line, " ")[1]
			break
		}
	}

	changedFiles := map[string]bool{}
	for k := range rawChanges {
		if strings.HasSuffix(k, ".go") {
			changedFiles[packageName+"/"+filepath.Dir(k)] = true
		}
	}

	for k, v := range toGoSum {
		if oldV, ok := fromGoSum[k]; !ok || oldV != v {
			changedFiles[k] = true
		}
	}

	fileIter := toTree.Files()
	die(err)
	fset := token.NewFileSet()
	hasChanged := true
	var files []*object.File
	die(fileIter.ForEach(func(file *object.File) error {
		if !strings.HasSuffix(file.Name, ".go") {
			return nil
		}
		content, err := file.Contents()
		if err != nil {
			return err
		}

		node, err := parser.ParseFile(fset, file.Name, content, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			return err
		}

		for _, cg := range node.Comments {
			for _, c := range cg.List {
				if strings.HasPrefix(c.Text, "// +build") {
					return nil
				}
			}
		}

		files = append(files, file)
		return nil
	}))
	deps := map[string][]string{}
	for hasChanged {
		hasChanged = false
		for _, file := range files {
			content, err := file.Contents()
			if err != nil {
				die(err)
			}

			node, err := parser.ParseFile(fset, file.Name, content, parser.ImportsOnly)
			if err != nil {
				die(err)
			}

			for _, i := range node.Imports {
				// remove quotes
				ref := i.Path.Value
				ref = ref[1:]
				ref = ref[:len(ref)-1]

				if changedFiles[ref] {
					deps[ref] = append(deps[ref], file.Name)
					name := filepath.Dir(file.Name)
					name = packageName + "/" + name
					if strings.HasSuffix(name, "/.") {
						name = name[:len(name)-2]
					}
					if !changedFiles[name] {
						hasChanged = true
					}
					changedFiles[name] = true
				}
			}
		}
	}

	for k := range changedFiles {
		if !strings.HasPrefix(k, packageName) {
			delete(changedFiles, k)
		}
	}

	for k := range changedFiles {
		fmt.Println(k)
		if *pExplain {
			for _, dep := range deps[k] {
				fmt.Println("    ", dep)
			}
		}
	}
}

func die(err error) {
	if err != nil {
		panic(err)
	}
}

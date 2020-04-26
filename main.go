package main

import (
	"flag"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

var (
	pRepo     = flag.String("repo", "", "repository url to be cloned")
	pBranch   = flag.String("branch", "", "branch to be cloned")
	pFromHash = flag.String("from", "", "from commit hash")
	pToHash   = flag.String("to", "", "to commit hash")
)

func parseFromHash(repo *git.Repository, hash string) (*object.Tree, map[string]string) {
	commit, err := repo.CommitObject(plumbing.NewHash(hash))
	die(err)

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
	repoURL := "https://github.com/dstreamcloud/diff-sample"
	branch := "go"
	fromHash := "789ba8319d640ec813c0fd007900b64d9ed9a3b1"
	toHash := "04cd22e0862ba14f10947fa4623c6b70a0cf37b0"

	repo, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
		URL:           repoURL,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
	})
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
		if from != nil {
			rawChanges[from.Path()] = true
		}
		if to != nil {
			rawChanges[to.Path()] = true
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
		files = append(files, file)
		return nil
	}))
	for hasChanged {
		hasChanged = false
		for _, file := range files {
			content, err := file.Contents()
			if err != nil {
				panic(err)
			}

			node, err := parser.ParseFile(fset, file.Name, content, parser.ImportsOnly)
			if err != nil {
				panic(err)
			}
			for _, i := range node.Imports {
				ref := i.Path.Value
				ref = ref[1:]
				ref = ref[:len(ref)-1]
				if changedFiles[ref] {
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
		println(k)
	}
}

func die(err error) {
	if err != nil {
		panic(err)
	}
}

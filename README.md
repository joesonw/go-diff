# Install

`go get github.com/joesonw/go-diff`

# Why

This little handy tool helps to manage large mono repositories. You don't have to rebuild every single service upon each little update.

This will help you to find out which services should be updated.

# Important
Only works with `GO MODULES` ATM

# Usage

`go-diff -repo github.com/user/repo -branch master -from <HASH> -to <HASH>`

# What does it do
1. It clones your repo
2. It then checkouts into specified branch.
3. It then does a diff between two hashes.
4. It will add any changed `.go` files to the change list
5. It compares two `go.sum` files and compute changed libraries, then added to change list
6. It parses every `.go` file and find import dependencies.
7. It adds every file that imported any package from change list, and does over and over again, until nothing new isfound.
8. TADA! You have a list of packages that needed to be rebuilt.

# Example

`https://github.com/dstreamcloud/diff-sample/tree/go`


## Example 1

> go-diff -repo https://github.com/dstreamcloud/diff-sample -branch go -from 1d748721adb49b79a529e446cca4b1a21bdd9b13 -to 4170f40939e939372e5c55beacd6a69ead53b976

This diff only changed `pkg/critical/critical.go`. But all packages are considered changed.

## Example 2
> go-diff -repo https://github.com/dstreamcloud/diff-sample -branch go -from 4170f40939e939372e5c55beacd6a69ead53b976 -to 6597f05c8b41f1182ad4fba09720728ada22a7fa

This diff only updated dependency of logrus. Thus only `pkg/logrus` and root which depends on it are considered changed.

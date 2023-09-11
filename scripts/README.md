# Scripts

This is the README for our scripts. To learn more about `routing-release`, go to the main [README](../README.md).

| Name | Purpose | Notes |
| --- | --- | --- |
| commit_with_shortlog | creates a commit with provided story number and a shortlog of all submodule changes | depends on staged_shortlog |
| light-commit-with-submodule-log | lightweight script for submodule bumps, allows for commits that don't finish a story | depends on submodule-log |
| push_with_recurse | pushes release and local submodule bumps whose commits not available on remote | |
| setup-git-hooks | installs git hooks from routing-release | |
| staged_shortlog | prints the submodule log titles for any staged submodule changes | |
| submodule-log | prints the cached submodule log and if you provide story id(s) will add finishes tag(s) | |
| sync-package-specs | updates the package spec file for component dependencies | |
| template-tests | runs the spec templating tests | |
| update | updates all submodules | |


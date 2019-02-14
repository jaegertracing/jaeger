# Github workflow for contributing to jaeger

Table of Contents

* [Fork a repository](#fork-a-repository)
* [Clone fork repository to local](#clone-fork-repository-to-local)
* [Create a branch to add a new feature or fix issues](#create-a-branch-to-add-a-new-feature-or-fix-issues)
* [Commit and Push](#commit-and-push)
* [Create a Pull Request](#create-a-pull-request)


The [jaeger](https://github.com/jaegertracing/jaeger.git) code is hosted on Github (https://github.com/jaegertracing/jaeger). The repository is called `upstream`. Contributors will develop and commit their changes in a clone of upstream repository. Then contributors push their change to their forked repository (`origin`) and create a Pull Request (PR), the PR will be merged to `upstream` repository if it meets the all the necessary requirement.		

## Fork a repository

 Goto https://github.com/jaegertracing/jaeger then hit the `Fork` button to fork your own copy of repo **jaeger** to your github account.

## Clone the forked repository to local

Clone the forked repo in [above step](#fork-a-repository) to your local working directory:
```sh
$ git clone https://github.com/$your_github_account/jaeger.git   
```

Keep your fork in sync with the main repo, add an `upstream` remote:
```sh
$ cd jaeger
$ git remote add upstream https://github.com/jaegertracing/jaeger.git
$ git remote -v

origin  https://github.com/$your_github_account/jaeger.git (fetch)
origin  https://github.com/$your_github_account/jaeger.git (push)
upstream        https://github.com/jaegertracing/jaeger.git (fetch)
upstream        https://github.com/jaegertracing/jaeger.git (push)
```

Sync your local `master` branch:
```sh
$ git checkout master
$ git pull upstream master
```

## Create a branch to add a new feature or fix issues

Before making any change, create a new branch:
```sh
$ git checkout master
$ git pull upstream master
$ git checkout -b new-feature
```

## Commit and Push

Make any change on the branch `new-feature`  then build and test your codes.  
Include in what will be committed:
```sh
$ git add <file>
```

Commit your changes with `sign-offs`
```sh
$ git commit -s
```

Enter your commit message to describe the changes. See the tips for a good commit message at [here](https://chris.beams.io/posts/git-commit/).  
Likely you go back and edit/build/test some more then `git commit --amend`  

Push your branch `new-feature` to your forked repository:
```sh
$ git push -u origin new-feature
```

## Create a Pull Request

* Goto your fork at https://github.com/$your_github_account/jaeger
* Create a Pull Request from the branch you recently pushed by hitting the button `Compare & pull request` next to branch name.

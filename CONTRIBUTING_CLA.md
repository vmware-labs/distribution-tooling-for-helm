# Contributing to distribution-tooling-for-helm

We welcome contributions from the community and first want to thank you for taking the time to contribute!

Please familiarize yourself with the [Code of Conduct](https://github.com/vmware/.github/blob/main/CODE_OF_CONDUCT.md) before contributing.

Before you start working with distribution-tooling-for-helm, please read and sign our Contributor License Agreement [CLA](https://cla.vmware.com/cla/1/preview). If you wish to contribute code and you have not signed our contributor license agreement (CLA), our bot will prompt you to do so when you open a Pull Request. For any questions about the CLA process, please refer to our [FAQ]([https://cla.vmware.com/faq](https://cla.vmware.com/faq)).

## Ways to contribute

We welcome many different types of contributions and not all of them need a Pull request. Contributions may include:

* New features and proposals
* Documentation
* Bug fixes
* Issue Triage
* Answering questions and giving feedback
* Helping to onboard new contributors
* Other related activities

## Getting started

First of all make sure you have read our [README](README.md) and specifically the [installation, downloading and building from source](https://github.com/vmware-labs/distribution-tooling-for-helm/tree/main#installation) sections.

For every contribution, you will have to make sure that all the tests pass. Moreover, consider adding new tests for any new functionality. You can run all the test by executing:

```
make test
```

Before sending any contribution is also a good practice to make sure that all code is formatted consistently:

```
make format
```

## Contribution Flow

This is a rough outline of what a contributor's workflow looks like:

- Create a topic branch from where you want to base your work
- Make commits of logical units
- Make sure your commit messages are in the proper format (see below)
- Push your changes to a topic branch in your fork of the repository
- Submit a pull request

Example:

``` shell
git remote add upstream https://github.com/vmware-labs/distribution-tooling-for-helm.git
git checkout -b my-new-feature main
git commit -a
git push origin my-new-feature
```

### Staying In Sync With Upstream

When your branch gets out of sync with the vmware-labs/main branch, use the following to update:

``` shell
git checkout my-new-feature
git fetch -a
git pull --rebase upstream main
git push --force-with-lease origin my-new-feature
```

### Updating pull requests

If your PR fails to pass CI or needs changes based on code review, you'll most likely want to squash these changes into
existing commits.

If your pull request contains a single commit or your changes are related to the most recent commit, you can simply
amend the commit.

``` shell
git add .
git commit --amend
git push --force-with-lease origin my-new-feature
```

If you need to squash changes into an earlier commit, you can use:

``` shell
git add .
git commit --fixup <commit>
git rebase -i --autosquash main
git push --force-with-lease origin my-new-feature
```

Be sure to add a comment to the PR indicating your new changes are ready to review, as GitHub does not generate a
notification when you git push.

### Pull Request Checklist

Before submitting your pull request, we advise you to use the following:

1. Check if your code changes will pass both code linting checks and unit tests.
2. Ensure your commit messages are descriptive. We follow the conventions on [How to Write a Git Commit Message](http://chris.beams.io/posts/git-commit/). Be sure to include any related GitHub issue references in the commit message. See [GFM syntax](https://guides.github.com/features/mastering-markdown/#GitHub-flavored-markdown) for referencing issues and commits.
3. Check the commits and commits messages and ensure they are free from typos.

## Release Process

All stable code is hosted at the main branch. Releases are done on demand through the Release GitHub workflow. In order to release the current HEAD, you will need to trigger this workflow passing the version being released (i.e. v0.2.0).

## Reporting Bugs and Creating Issues

For specifics on what to include in your report, please follow the guidelines in the issue and pull request templates when available. Try to roughly follow the commit message format conventions above.

## Ask for Help

The best way to reach us with a question when contributing is by creating a new issue on the [GitHub issues](https://github.com/vmware-labs/distribution-tooling-for-helm/issues) section.

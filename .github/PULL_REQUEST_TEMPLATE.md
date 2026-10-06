## Summary

What does this change do, and why?

Closes #

> For anything beyond a small fix, link the issue where the approach was agreed.
> The pull request title becomes the squash commit title: use Conventional Commits.

## Checklist

- [ ] Every commit is signed off (`git commit -s`) under the [Developer Certificate of Origin](https://developercertificate.org).
- [ ] Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/).
- [ ] `go test ./...`, `go vet ./...` and `golangci-lint run` pass; `gofmt -l .` prints nothing.
- [ ] Tests cover the change (or explain why none are needed).
- [ ] Documentation and `openspec/specs` are updated where behavior changed.
- [ ] I understand every line of this change, including any part written with AI tools.

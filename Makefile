ANSI_BOLD := $(if $NO_COLOR,$(shell tput bold 2>/dev/nul),)
ANSI_RESET := $(if $NO_COLOR,$(shell tput sgr0 2>/dev/nul),)

# Switch this to podman if you are using that in place of docker
CONTAINERTOOL := docker

VERSION := $(if $(shell git status --porcelain 2>/dev/null),latest,$(shell git rev-parse HEAD))

##@ Build

.PHONY: build
build: .cloudbees/testing/action.yml ## Build the container image
	@echo "⚡️ Building container image ..."
	@$(CONTAINERTOOL) build --rm -t checkout-action:$(VERSION) -f Dockerfile .
	@echo "✅ Container image built"

.PHONY: test
test: ## Runs unit tests
	@echo "⚡️ Running unit tests ..."
	@go test ./...
	@echo "✅ Unit tests passed"

.PHONY: verify
verify: format sync test ## Verifies that the committed code is formatted, all files are in sync and the unit tests pass
	@if [ "`git status --porcelain 2>/dev/null`x" = "x" ] ; then \
	  echo "✅ Git workspace is clean" ; \
	else \
	  echo "❌ Git workspace is dirty" ; \
	  exit 1 ; \
	fi

manual-test: build ## Interactively runs the test container to allow manual testing
	@echo 'ℹ️  To test, run something like:'
	@echo '       checkout \'
	@echo '            --provider github \'
	@echo '            --repository example/repoName \'
	@echo '            --token ...your.token.here...'
	@$(CONTAINERTOOL) run --rm -ti --entrypoint sh --workdir /cloudbees/workspace -e CLOUDBEES_WORKSPACE=/cloudbees/workspace checkout-action:$(VERSION)

.cloudbees/testing/action.yml: action.yml Makefile ## Ensures that the test version of the action.yml is in sync with the production version
	@echo "⚡️ Updating $@ ..."
	@sed -e 's|docker://public.ecr.aws/l7o7z1g8/actions/|docker://020229604682.dkr.ecr.us-east-1.amazonaws.com/actions/|g' < action.yml > .cloudbees/testing/action.yml

.cloudbees/workflows/workflow.yml: Dockerfile ## Ensures that the workflow uses the same version of go as the Dockerfile
	@echo "⚡️ Updating $@ ..."
	@IMAGE=$$(sed -ne 's/FROM[ \t]*golang:\([^ \t]*\)-alpine[0-9.]*[ \t].*/\1/p' Dockerfile) ; \
	sed -e 's|\(uses:[ \t]*docker://golang:\)[^ \t]*|\1'"$$IMAGE"'|;' < $@ > $@.bak ; \
	mv -f $@.bak $@

.PHONY: sync
sync: .cloudbees/testing/action.yml .cloudbees/workflows/workflow.yml ## Updates action.yml so that the container tag matches the VERSION file
	@echo "✅ All files synchronized"

.PHONY: format
format: ## Applies the project code style
	@echo "⚡️ Applying project code style ..."
	@gofmt -w .
	@echo "✅ Project code style applied"

##@ Release

.PHONY: -check-main-matches-remote
-check-main-matches-remote:
	@echo "⚡️ Checking local 'main' branch against remote ..."
	@git fetch origin --force --tags main 2>/dev/null && \
	[ "$$(git rev-parse main)" = "$$(git rev-parse origin/main)" ] && \
	echo "✅ Remote 'main' branch matches local 'main' branch" || \
	( echo "❌ Remote 'main' branch does not match local 'main' branch" ; exit 1 )

.PHONY: -check-main-already-tagged
-check-main-already-tagged: -check-main-matches-remote
	@if [ "`git status --porcelain 2>/dev/null`x" = "x" ] ; then \
	  echo "✅ Git workspace is clean" ; \
	else \
	  echo "❌ Must be in a clean Git workspace to run this target" ; \
	  exit 1 ; \
	fi
	@[ "$$(git rev-parse main)" = "$$(git rev-parse HEAD)" ] && \
	echo "❌ Must be on 'main' branch to run this target " && exit 1 ; \
	echo "✅ On 'main' branch"
	@LAST_VERSION="$$(git describe --tags --match 'v*.*.*' --exact-match main 2>/dev/null | sed -e 's:^tags/::')" ; \
	[ "$$(git rev-parse main)" = "$$(git rev-parse "$${LAST_VERSION}^{commit}"  2>/dev/null)" ] && \
	echo "❌ Tags for 'main' were already created as version $$LAST_VERSION" && exit 1 ; \
	echo "✅ Lastest 'main' branch has not been tagged yet"

.PHONY: preview-patch-release
preview-patch-release: -check-main-already-tagged ## Displays the next a patch release from the main branch
	@echo "ℹ️  Next patch release version $$(go run .cloudbees/release/next-version.go)"

.PHONY: preview-minor-release
preview-minor-release: -check-main-already-tagged ## Displays the next a minor release from the main branch
	@echo "ℹ️  Next patch release version $$(go run .cloudbees/release/next-version.go)"

.PHONY: preview-major-release
preview-major-release: -check-main-already-tagged ## Displays the next a major release from the main branch
	@echo "ℹ️  Next patch release version $$(go run .cloudbees/release/next-version.go)"

.PHONY: prepare-patch-release
prepare-patch-release: -check-main-already-tagged ## Creates a tag for a patch release from the main branch
	@NEXT_VERSION="$$(go run .cloudbees/release/next-version.go)" ; \
	echo "⚡️ Tagging version $$NEXT_VERSION ..." ; \
	git tag -f -a -m "chore: $$NEXT_VERSION release" $$NEXT_VERSION main ; \
	echo "✅ Version $$NEXT_VERSION tagged from branch 'main'"

.PHONY: prepare-minor-release
prepare-minor-release: -check-main-already-tagged ## Creates a tag for a minor release from the main branch
	@NEXT_VERSION="$$(go run .cloudbees/release/next-version.go --minor)" ; \
	echo "⚡️ Tagging version $$NEXT_VERSION ..." ; \
	git tag -f -s -m "chore: $$NEXT_VERSION release" $$NEXT_VERSION main ; \
	echo "✅ Version $$NEXT_VERSION tagged from branch 'main'"

.PHONY: prepare-major-release
prepare-major-release: -check-main-already-tagged ## Creates a tag for a major release from the main branch
	@NEXT_VERSION="$$(go run .cloudbees/release/next-version.go --major)" ; \
	echo "⚡️ Tagging version $$NEXT_VERSION ..." ; \
	git tag -f -s -m "chore: $$NEXT_VERSION release" $$NEXT_VERSION main ; \
	echo "✅ Version $$NEXT_VERSION tagged from branch 'main'"

.PHONY: publish-release
publish-release: ## Pushes the latest release tag for the current commit's release tag
	@CUR_VERSION="$$(git describe --tags --match 'v*.*.*' --exact-match 2>/dev/null | sed -e 's:^tags/::')" ; \
	if [ -z "$${CUR_VERSION}" ] ; \
	then \
		echo "❌ Current commit does not have a release tag" ; \
		exit 1 ; \
	fi ; \
	echo "⚡️ Publishing current commit's release tag $$CUR_VERSION ..." ; \
	git push --force origin $$CUR_VERSION ; \
	echo "✅ Release tag $$CUR_VERSION published"

.PHONY: prepare-floating-tags
prepare-floating-tags: ## Synchronizes the floating tags with the current commit's release tag
	@CUR_VERSION="$$(git describe --tags --match 'v*.*.*' --exact-match 2>/dev/null | sed -e 's:^tags/::')" ; \
	if [ -z "$${CUR_VERSION}" ] ; \
	then \
		echo "❌ Current commit does not have a release tag" ; \
		exit 1 ; \
	fi ; \
	version="$$CUR_VERSION" ; \
	echo "⚡️ Current commit has release tag $$CUR_VERSION, updating floating tags to match ..." ; \
	while echo "$$version" | grep -q '\.[0-9][0-9]*$$' ; \
	do \
		version=$$(echo "$$version" | sed 's/\.[0-9][0-9]*$$//') ; \
		if [ "$$(git rev-parse "$$version^{commit}" 2>/dev/null)" != "$$(git rev-parse "$${CUR_VERSION}^{commit}")" ] ; then \
			git tag -f -s -m "chore: $$CUR_VERSION release" $$version $$CUR_VERSION ; \
		fi ; \
	done ; \
	echo "✅ Floating tags updated to match release tag $$CUR_VERSION"

.PHONY: publish-floating-tags
publish-floating-tags: ## Pushes any floating tags that match the current commit's release tag
	@CUR_VERSION="$$(git describe --tags --match 'v*.*.*' --exact-match main 2>/dev/null | sed -e 's:^tags/::')" ; \
	if [ -z "$${LAST_VERSION}" ] ; \
	then \
		echo "❌ Current commit does not have a release tag" ; \
		exit 1 ; \
	fi ; \
	echo "⚡️ Looking for floating tags that point to $$LAST_VERSION ..." ; \
	tags="" ; \
	version="$$LAST_VERSION" ; \
	while echo "$$version" | grep -q '\.[0-9][0-9]*$$' ; \
	do \
		version=$$(echo "$$version" | sed 's/\.[0-9][0-9]*$$//') ; \
		if [ "$$(git rev-parse "$$version^{commit}" 2>/dev/null)" = "$$(git rev-parse "$${LAST_VERSION}^{commit}")" ] ; then \
			tags="$$tags $$version" ; \
		fi ; \
	done ; \
	if [ -z "$tags" ] ; then \
		echo "✅ No floating tags point to the current version $$LAST_VERSION" ; \
	else \
		echo "⚡️ Publishing floating tag(s)$$tags ..." ; \
		git push --force origin$$tags ; \
		echo "✅ Floating tag(s)$$tags published" ; \
	fi

##@ Miscellaneous

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


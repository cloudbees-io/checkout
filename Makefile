ANSI_BOLD := $(if $NO_COLOR,$(shell tput bold 2>/dev/null),)
ANSI_RESET := $(if $NO_COLOR,$(shell tput sgr0 2>/dev/null),)

# Switch this to podman if you are using that in place of docker
CONTAINERTOOL := docker

VERSION := $(if $(shell git status --porcelain 2>/dev/null),latest,$(shell git rev-parse HEAD))

##@ Build

.PHONY: build
build: .cloudbees/testing/action.yml ## Build the container image
	@echo "$(ANSI_BOLD)⚡️ Building container image ...$(ANSI_RESET)"
	@$(CONTAINERTOOL) build --rm -t checkout-action:$(VERSION) -f Dockerfile .
	@echo "$(ANSI_BOLD)✅ Container image built$(ANSI_RESET)"

.PHONY: test
test: ## Runs unit tests
	@echo "$(ANSI_BOLD)⚡️ Running unit tests ...$(ANSI_RESET)"
	@go test ./...
	@echo "$(ANSI_BOLD)✅ Unit tests passed$(ANSI_RESET)"

.PHONY: verify
verify: format sync test ## Verifies that the committed code is formatted, all files are in sync and the unit tests pass
	@if [ "`git status --porcelain 2>/dev/null`x" = "x" ] ; then \
	  echo "$(ANSI_BOLD)✅ Git workspace is clean$(ANSI_RESET)" ; \
	else \
	  echo "$(ANSI_BOLD)❌ Git workspace is dirty$(ANSI_RESET)" ; \
	  git status --porcelain ; \
	  git diff ; \
	  exit 1 ; \
	fi

manual-test: build ## Interactively runs the test container to allow manual testing
	@echo '$(ANSI_BOLD)ℹ️  To test, run something like:$(ANSI_RESET)'
	@echo '       checkout \'
	@echo '            --provider github \'
	@echo '            --repository example/repoName \'
	@echo '            --token ...your.token.here...'
	@$(CONTAINERTOOL) run --rm -ti --entrypoint sh --workdir /cloudbees/workspace -e CLOUDBEES_WORKSPACE=/cloudbees/workspace checkout-action:$(VERSION)

.cloudbees/testing/action.yml: action.yml Makefile ## Ensures that the test version of the action.yml is in sync with the production version
	@echo "$(ANSI_BOLD)⚡️ Updating $@ ...$(ANSI_RESET)"
	@sed -e 's|docker://public.ecr.aws/l7o7z1g8/actions/|docker://020229604682.dkr.ecr.us-east-1.amazonaws.com/actions/|g' < action.yml > .cloudbees/testing/action.yml

.cloudbees/workflows/workflow.yml: Dockerfile ## Ensures that the workflow uses the same version of go as the Dockerfile
	@echo "$(ANSI_BOLD)⚡️ Updating $@ ...$(ANSI_RESET)"
	@IMAGE=$$(sed -ne 's/FROM[ \t]*golang:\([^ \t]*\)-alpine[0-9.]*[ \t].*/\1/p' Dockerfile) ; \
	sed -e 's|\(uses:[ \t]*docker://golang:\)[^ \t]*|\1'"$$IMAGE"'|;' < $@ > $@.bak ; \
	mv -f $@.bak $@

.PHONY: sync
sync: .cloudbees/testing/action.yml .cloudbees/workflows/workflow.yml ## Updates action.yml so that the container tag matches the VERSION file
	@echo "$(ANSI_BOLD)✅ All files synchronized$(ANSI_RESET)"

.PHONY: format
format: ## Applies the project code style
	@echo "$(ANSI_BOLD)⚡️ Applying project code style ...$(ANSI_RESET)"
	@gofmt -w .
	@echo "$(ANSI_BOLD)✅ Project code style applied$(ANSI_RESET)"

##@ Release

.PHONY: -check-main-matches-remote
-check-main-matches-remote:
	@echo "$(ANSI_BOLD)⚡️ Checking local 'main' branch against remote ...$(ANSI_RESET)"
	@git fetch origin --force --tags main 2>/dev/null && \
	if [ "$$(git rev-parse main)" = "$$(git rev-parse origin/main)" ] ; then \
	  echo "$(ANSI_BOLD)✅ Remote 'main' branch matches local 'main' branch$(ANSI_RESET)" ; \
	else \
	  echo "$(ANSI_BOLD)❌ Remote 'main' branch does not match local 'main' branch$(ANSI_RESET)" ; \
	  exit 1 ; \
	fi

.PHONY: -check-main-already-tagged
-check-main-already-tagged: -check-main-matches-remote
	@if [ "`git status --porcelain 2>/dev/null`x" = "x" ] ; then \
	  echo "$(ANSI_BOLD)✅ Git workspace is clean$(ANSI_RESET)" ; \
	else \
	  echo "$(ANSI_BOLD)❌ Must be in a clean Git workspace to run this target$(ANSI_RESET)" ; \
	  exit 1 ; \
	fi
	@if [ "$$(git rev-parse main)" = "$$(git rev-parse HEAD)" ] ; then \
	  echo "$(ANSI_BOLD)✅ On 'main' branch$(ANSI_RESET)" ; \
	else \
	  echo "$(ANSI_BOLD)❌ Must be on 'main' branch to run this target $(ANSI_RESET)" ; \
	  exit 1 ; \
	fi
	@LAST_VERSION="$$(git describe --tags --match 'v*.*.*' --exact-match main 2>/dev/null | sed -e 's:^tags/::')" ; \
	if [ "$$(git rev-parse main)" = "$$(git rev-parse "$${LAST_VERSION}^{commit}"  2>/dev/null)" ] ; then \
	  echo "$(ANSI_BOLD)❌ Tags for 'main' were already created as version $$LAST_VERSION$(ANSI_RESET)" ; \
	  exit 1 ; \
	else \
	  echo "$(ANSI_BOLD)✅ Lastest 'main' branch has not been tagged yet$(ANSI_RESET)" ; \
	fi

.PHONY: preview-patch-release
preview-patch-release: -check-main-already-tagged ## Displays the next a patch release from the main branch
	@echo "$(ANSI_BOLD)ℹ️  Next patch release version $$(go run .cloudbees/release/next-version.go)$(ANSI_RESET)"

.PHONY: preview-minor-release
preview-minor-release: -check-main-already-tagged ## Displays the next a minor release from the main branch
	@echo "$(ANSI_BOLD)ℹ️  Next patch release version $$(go run .cloudbees/release/next-version.go)$(ANSI_RESET)"

.PHONY: preview-major-release
preview-major-release: -check-main-already-tagged ## Displays the next a major release from the main branch
	@echo "$(ANSI_BOLD)ℹ️  Next patch release version $$(go run .cloudbees/release/next-version.go)$(ANSI_RESET)"

.PHONY: prepare-patch-release
prepare-patch-release: -check-main-already-tagged ## Creates a tag for a patch release from the main branch
	@NEXT_VERSION="$$(go run .cloudbees/release/next-version.go)" ; \
	echo "$(ANSI_BOLD)⚡️ Tagging version $$NEXT_VERSION ...$(ANSI_RESET)" ; \
	git tag -f -a -m "chore: $$NEXT_VERSION release" $$NEXT_VERSION main ; \
	echo "$(ANSI_BOLD)✅ Version $$NEXT_VERSION tagged from branch 'main'$(ANSI_RESET)"

.PHONY: prepare-minor-release
prepare-minor-release: -check-main-already-tagged ## Creates a tag for a minor release from the main branch
	@NEXT_VERSION="$$(go run .cloudbees/release/next-version.go --minor)" ; \
	echo "$(ANSI_BOLD)⚡️ Tagging version $$NEXT_VERSION ...$(ANSI_RESET)" ; \
	git tag -f -s -m "chore: $$NEXT_VERSION release" $$NEXT_VERSION main ; \
	echo "$(ANSI_BOLD)✅ Version $$NEXT_VERSION tagged from branch 'main'$(ANSI_RESET)"

.PHONY: prepare-major-release
prepare-major-release: -check-main-already-tagged ## Creates a tag for a major release from the main branch
	@NEXT_VERSION="$$(go run .cloudbees/release/next-version.go --major)" ; \
	echo "$(ANSI_BOLD)⚡️ Tagging version $$NEXT_VERSION ...$(ANSI_RESET)" ; \
	git tag -f -s -m "chore: $$NEXT_VERSION release" $$NEXT_VERSION main ; \
	echo "$(ANSI_BOLD)✅ Version $$NEXT_VERSION tagged from branch 'main'$(ANSI_RESET)"

.PHONY: publish-release
publish-release: ## Pushes the latest release tag for the current commit's release tag
	@CUR_VERSION="$$(git describe --tags --match 'v*.*.*' --exact-match 2>/dev/null | sed -e 's:^tags/::')" ; \
	if [ -z "$${CUR_VERSION}" ] ; \
	then \
		echo "$(ANSI_BOLD)❌ Current commit does not have a release tag$(ANSI_RESET)" ; \
		exit 1 ; \
	fi ; \
	echo "$(ANSI_BOLD)⚡️ Publishing current commit's release tag $$CUR_VERSION ...$(ANSI_RESET)" ; \
	git push --force origin $$CUR_VERSION ; \
	echo "$(ANSI_BOLD)✅ Release tag $$CUR_VERSION published$(ANSI_RESET)"

.PHONY: prepare-floating-tags
prepare-floating-tags: ## Synchronizes the floating tags with the current commit's release tag
	@CUR_VERSION="$$(git describe --tags --match 'v*.*.*' --exact-match 2>/dev/null | sed -e 's:^tags/::')" ; \
	if [ -z "$${CUR_VERSION}" ] ; \
	then \
		echo "$(ANSI_BOLD)❌ Current commit does not have a release tag$(ANSI_RESET)" ; \
		exit 1 ; \
	fi ; \
	version="$$CUR_VERSION" ; \
	echo "$(ANSI_BOLD)⚡️ Current commit has release tag $$CUR_VERSION, updating floating tags to match ...$(ANSI_RESET)" ; \
	while echo "$$version" | grep -q '\.[0-9][0-9]*$$' ; \
	do \
		version=$$(echo "$$version" | sed 's/\.[0-9][0-9]*$$//') ; \
		if [ "$$(git rev-parse "$$version^{commit}" 2>/dev/null)" != "$$(git rev-parse "$${CUR_VERSION}^{commit}")" ] ; then \
			git tag -f -s -m "chore: $$CUR_VERSION release" $$version "$$CUR_VERSION^{}" ; \
		fi ; \
	done ; \
	echo "$(ANSI_BOLD)✅ Floating tags updated to match release tag $$CUR_VERSION$(ANSI_RESET)"

.PHONY: publish-floating-tags
publish-floating-tags: ## Pushes any floating tags that match the current commit's release tag
	@CUR_VERSION="$$(git describe --tags --match 'v*.*.*' --exact-match main 2>/dev/null | sed -e 's:^tags/::')" ; \
	if [ -z "$${CUR_VERSION}" ] ; \
	then \
		echo "$(ANSI_BOLD)❌ Current commit does not have a release tag$(ANSI_RESET)" ; \
		exit 1 ; \
	fi ; \
	echo "$(ANSI_BOLD)⚡️ Looking for floating tags that point to $$CUR_VERSION ...$(ANSI_RESET)" ; \
	tags="" ; \
	version="$$CUR_VERSION" ; \
	while echo "$$version" | grep -q '\.[0-9][0-9]*$$' ; \
	do \
		version=$$(echo "$$version" | sed 's/\.[0-9][0-9]*$$//') ; \
		if [ "$$(git rev-parse "$$version^{commit}" 2>/dev/null)" = "$$(git rev-parse "$${CUR_VERSION}^{commit}")" ] ; then \
			tags="$$tags $$version" ; \
		fi ; \
	done ; \
	if [ -z "$$tags" ] ; then \
		echo "$(ANSI_BOLD)✅ No floating tags point to the current version $$CUR_VERSION$(ANSI_RESET)" ; \
	else \
		echo "$(ANSI_BOLD)⚡️ Publishing floating tag(s)$$tags ...$(ANSI_RESET)" ; \
		git push --force origin$$tags ; \
		echo "$(ANSI_BOLD)✅ Floating tag(s)$$tags published$(ANSI_RESET)" ; \
	fi

##@ Miscellaneous

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


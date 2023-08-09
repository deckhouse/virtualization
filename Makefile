SHELL := /bin/bash

.PHONY: update-patch
update-patch: check-yq
	make bump version=$$(./hack/increase_semver.sh -p $$(yq .version Chart.yaml))

.PHONY: update-minor
update-minor: check-yq
	make bump version=$$(./hack/increase_semver.sh -m $$(yq .version Chart.yaml))

.PHONY: update-major
update-major: check-yq
	make bump version=$$(./hack/increase_semver.sh -M $$(yq .version Chart.yaml))

.PHONY: bump
bump: current-version
	yq -i '.version="$(version)"' Chart.yaml
	yq -i '.version="v$(version)"' release.yaml

	jq --arg now $(shell date +%FT%T%z) '.version="$(version)" | .buildDateTime=$$now' images/frontend/package.json > /tmp/frontend_package.json
	cp /tmp/frontend_package.json images/frontend/package.json
	# Updating version in package-lock.json to avoid future diffs on `npm i`
	pushd images/frontend/ && npm i ; popd

	git commit -a -s -m "bump version $(version)"
	git tag "v$(version)"

.PHONY: push
push:
	git push -u origin HEAD && git push --tags

.PHONY: current-version
current-version: check-yq
	@echo "Current version: $$(yq .version Chart.yaml)"

.PHONY: check-yq
check-yq:
	@which yq >/dev/null || (echo "yq not found. Install it to change Chart.yaml"; exit 1)

.PHONY: check-jq
check-jq:
	@which jq >/dev/null || (echo "jq not found. Install it to change package.json"; exit 1)

##@ Helm lib

.PHONY: helm-update-subcharts
helm-update-subcharts: ## Download subcharts into charts directory. Please, set desired versions in Chart.yaml before download.
	@which helm || (echo "Helm not found. Please, install helm to update helm_lib."; exit 1)
	helm repo add deckhouse https://deckhouse.github.io/lib-helm && \
	helm repo update && \
  	helm dep update

.PHONY: helm-bump-helm-lib
helm-bump-helm-lib: ## Update helm lib in charts directory to specified version.
	##~ Options: version=<helm-lib semver, e.g 1.1.3>
	@which yq || (echo "yq not found. Install it to change Chart.yaml"; exit 1)
	yq -i '.dependencies[] |= select(.name == "deckhouse_lib_helm").version = "$(version)"' Chart.yaml
	git rm charts/*.tgz || true
	mkdir -p charts
	$(MAKE) helm-update-subcharts
	@echo "Helm lib updated to $(version)"
	ls -la charts

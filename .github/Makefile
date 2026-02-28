.PHONY: update
update:
	@echo "Updating dependencies..."
	@cd ../ && go get -u ./...
	@go mod tidy

.PHONY: update-linters
update-linters:
	@echo "Updating linters..."
	@go install mvdan.cc/gofumpt@latest
	@go install github.com/daixiang0/gci@latest
	@go install github.com/segmentio/golines@latest
	@go install github.com/google/addlicense@latest
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest

.PHONY: fmt
fmt:
	@echo "Formatting ..."
	@go mod tidy
	@go fmt ../...
	@golines -m 120 -t 4 -w ../
	@gofumpt -w -extra ../
	@gci write -s standard -s default -s "Prefix($(shell go list -m))" -s blank -s dot -s alias --skip-generated ../
	@fieldalignment -fix ../...

.PHONY: license
license:
	@echo "Checking License headers ..."
	@if addlicense -check -v -skip yaml -f licence-header.tmpl ../*; then echo "License headers OK"; else return 1; fi;

.PHONY: lint
lint: fmt license
	@echo "Linting ..."
	@if golangci-lint config verify -v --config=.github/.golangci.yml; then echo "Config OK"; else return 1; fi;
	@if golangci-lint run --config=.github/.golangci.yml ../...; then echo "Linting OK"; else return 1; fi;

.PHONY: vulncheck
vulncheck:
	@echo "Running govulncheck ..."
	@govulncheck -show verbose ../...

.PHONY: test
test:
	@echo "Running all tests ..."
	@go test -v -vet=all ../...

.PHONY: cover
cover:
	@echo "Testing with coverage ..."
	@go test -v -race -covermode=atomic -coverpkg=../... -coverprofile=./coverage.out ../...

.PHONY: fuzz
fuzz:
	@echo "Running fuzz tests ..."
	@fuzzTime=$${FUZZTIME:-10s}; \
	repo_root=$$(cd .. && pwd); \
	gocache_dir="$$repo_root/.gocache"; \
	gomodcache_dir=$$(go env GOMODCACHE); \
	mkdir -p "$$gocache_dir"; \
	cd "$$repo_root"; \
	targets_file=$$(mktemp); \
	trap 'rm -f "$$targets_file"' EXIT INT TERM; \
		for pkg in $$(GOCACHE="$$gocache_dir" GOMODCACHE="$$gomodcache_dir" go list ./...); do \
			fuzz_targets=$$(GOCACHE="$$gocache_dir" GOMODCACHE="$$gomodcache_dir" go test "$$pkg" -list '^Fuzz' | grep '^Fuzz' || true); \
		for func in $$fuzz_targets; do \
			printf '%s|%s\n' "$$pkg" "$$func" >> "$$targets_file"; \
		done; \
	done; \
	num_targets=$$(wc -l < "$$targets_file" | tr -d ' '); \
	if [ "$$num_targets" -eq 0 ]; then \
		echo "No fuzz targets found."; \
		exit 0; \
	fi; \
	case "$$fuzzTime" in \
		*h) fuzz_seconds=$$(($${fuzzTime%h} * 3600));; \
		*m) fuzz_seconds=$$(($${fuzzTime%m} * 60));; \
		*s) fuzz_seconds=$$(($${fuzzTime%s}));; \
		*) fuzz_seconds=$$(($$fuzzTime));; \
	esac; \
	total_seconds=$$((num_targets * fuzz_seconds)); \
	echo ""; \
	echo "Fuzz targets to run:"; \
	while IFS='|' read -r pkg func; do \
		echo "  - $$pkg :: $$func"; \
	done < "$$targets_file"; \
	echo ""; \
	echo "Number of targets: $$num_targets"; \
	echo "Time per target: $$fuzzTime"; \
	echo "Estimated total time: $$total_seconds seconds"; \
	echo ""; \
	failed=0; \
	current=0; \
	remaining_seconds=$$total_seconds; \
	while IFS='|' read -r pkg func; do \
		[ -n "$$pkg" ] || continue; \
		current=$$((current + 1)); \
		echo ""; \
		echo "[$$current/$$num_targets] Fuzzing $$pkg :: $$func"; \
		pkg_dir=$$(GOCACHE="$$gocache_dir" GOMODCACHE="$$gomodcache_dir" go list -f '{{.Dir}}' "$$pkg"); \
		testdata_dir="$$pkg_dir/testdata/fuzz/$$func"; \
		if [ -d "$$testdata_dir" ]; then \
			preexisting_dir=1; \
		else \
			preexisting_dir=0; \
			mkdir -p "$$testdata_dir"; \
		fi; \
		if ! GOCACHE="$$gocache_dir" GOMODCACHE="$$gomodcache_dir" go test "$$pkg" -run='^$$' -fuzz="^$${func}$$" -fuzztime="$$fuzzTime"; then \
			failed=1; \
			latest_corpus_file=$$(ls -t "$$testdata_dir" 2>/dev/null | head -n 1 || true); \
			if [ -n "$$latest_corpus_file" ]; then \
				echo "Latest corpus/crasher file for $$pkg :: $$func: $$testdata_dir/$$latest_corpus_file"; \
				cat "$$testdata_dir/$$latest_corpus_file"; \
			else \
				echo "Fuzzing failed for $$pkg :: $$func, but no corpus/crasher file was written (run may have been interrupted)."; \
			fi; \
		else \
			echo "No crasher found for $$pkg :: $$func."; \
		fi; \
		if [ $$preexisting_dir -eq 0 ] && [ -d "$$testdata_dir" ] && [ -z "$$(ls -A "$$testdata_dir")" ]; then \
			rmdir "$$testdata_dir"; \
		fi; \
		remaining_seconds=$$((remaining_seconds - fuzz_seconds)); \
		if [ $$remaining_seconds -gt 0 ]; then \
			echo ""; \
			echo "Estimated remaining time: $$remaining_seconds seconds"; \
		fi; \
	done < "$$targets_file"; \
	exit $$failed

.PHONY: bench
bench:
	@echo "Running benchmarks ..."
	@bench_regex=$${BENCH:-'.*Suites'}; \
	bench_time=$${BENCHTIME:-5s}; \
	go test -run '^$$' -bench "$$bench_regex" -benchmem -benchtime "$$bench_time" ../tests

.PHONY: release
release:
	@echo "Creating a new release ..."
	@tag=$${tag:-}; \
		if [ -z "$$tag" ]; then \
			echo "Error: tag variable is not set. Use 'make release tag=vX.Y.Z'"; \
			exit 1; \
		fi; \
		git fetch origin; \
	if ! git tag -s -a "$$tag" -m "chore: cut $$tag"; then \
		echo "Error: Failed to create tag"; \
		exit 1; \
	fi; \
	git push origin "$$tag"

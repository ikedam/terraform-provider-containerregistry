RM = rm -f
LOCKFILE = .terraform.lock.hcl

ifeq ($(OS),Windows_NT)
	RM = cmd.exe /C del
endif

.PHONY: help
help:	## Show target helps
	@echo "set ENV variable and call targets:"
	@echo
	@grep -E '^[a-zA-Z_%-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\t\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: init
init:	## Run terraform init
	$(RM) $(LOCKFILE)
	docker compose run --rm --build terraform init

.PHONY: plan
plan:	## Run terraform plan
	docker compose run --rm terraform plan

.PHONY: apply
apply:	## Run terraform apply
	docker compose run --rm terraform apply

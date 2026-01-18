# mainly for local testings
RM = rm -f
LOCKFILE = localtest/.terraform.lock.hcl

ifeq ($(OS),Windows_NT)
	RM = cmd.exe /C del
	LOCKFILE = localtest\\.terraform.lock.hcl
endif

.PHONY: help
help:	## Show target helps
	@echo "set ENV variable and call targets:"
	@echo
	@grep -E '^[a-zA-Z_%-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\t\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: init
init:	## Run terraform init in localtest
	$(RM) $(LOCKFILE)
	docker compose run --rm --build localtest init

.PHONY: plan
plan:	## Run terraform plan in localtest
	docker compose run --rm localtest plan

.PHONY: apply
apply:	## Run terraform apply in localtest
	docker compose run --rm localtest apply

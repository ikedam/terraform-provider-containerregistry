FROM golang:1.24.9-bookworm AS builder

WORKDIR /workspace
COPY . /workspace
RUN CGO_ENABLED=0 go build -o /terraform-provider-containerregistry main.go

FROM debian:bookworm-slim AS terraform

ARG TERRAFORM_VERSION
# decide platform name amd64 / arm64 automatically
RUN apt-get update \
  && apt-get install -y unzip curl \
  && mkdir /tmp/terraform \
  && curl -s -L -o /tmp/terraform/terraform.zip "https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_$(uname -m|sed 's/x86_64/amd64/').zip" \
  && unzip /tmp/terraform/terraform.zip -d /tmp/terraform \
  && mv /tmp/terraform/terraform /terraform \
  && rm -rf /tmp/terraform

FROM debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && apt-get clean \
  && rm -rf /var/lib/apt/lists/* \
  && echo 'provider_installation {' > ~/.terraformrc \
  && echo '  dev_overrides {' >> ~/.terraformrc \
  && echo '    "tf-containerregistry.ikedam.jp/ikedam/containerregistry" = "/providers"' >> ~/.terraformrc \
  && echo '  }' >> ~/.terraformrc \
  && echo '  direct {}' >> ~/.terraformrc \
  && echo '}' >> ~/.terraformrc \
  && chmod 644 ~/.terraformrc
COPY --from=terraform /terraform /terraform
COPY --from=builder /terraform-provider-containerregistry /providers/terraform-provider-containerregistry
ENTRYPOINT ["/terraform"]

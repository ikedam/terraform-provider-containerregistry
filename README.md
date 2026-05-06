# containerregistry Terraform プロバイダー

## 概要

containerregistry Terraform プロバイダーは、
Container Registry (Docker Registry) 上のイメージの作成を行います。
以下の場合にイメージをビルドして push します:

* 指定されたイメージが存在しない場合
* イメージのラベルに変更がある場合
* その他、トリガー条件に変更があった場合

**ビルド指定に変更があっても再ビルドは行わないので注意してください**

## 利用例

[example/](./example/)

## containerregistry_compose リソース

```hcl
terraform {
  required_providers {
    containerregistry = {
      source = "tf-containerregistry.ikedam.jp/ikedam/containerregistry"
      # 最新のバージョンは https://github.com/ikedam/terraform-provider-containerregistry/releases から確認してください。
      # version = "~> 0.3.0"
    }
  }
}

provider "containerregistry" {
  # buildx プラグインがインストールされていない場合、自動でインストールします。
  # compose v5 を使用する場合、 buildx プラグインの利用がデフォルトになるため、
  # buildx プラグインがインストールされているか、環境変数 DOCKER_BUILDKIT=0 を指定する必要があります。
  buildx_install_if_missing = true

  # buildx プラグインのバージョンを指定します。
  # デフォルトは最新のバージョンです。
  buildx_version = "v0.12.0"

  # プライベートレジストリー向けのユーザー名・パスワード (トークン) を指定します。
  # 詳細は後述の「認証」を参照してください。
  registry_auth = {
    "your.registry.host" = {
      username = "..."
      password = "..."
    }
  }
}

resource "containerregistry_compose" "app" {
  # 作成および push するイメージ URI を指定します。
  # タグ部分を変数にすることで、タグの変更時にイメージを再作成するなどの動作に指定できます。
  image_uri = "your.image.registry/repository:v0.0.0"

  # build には、 docker compose v5 互換のビルド指定を記述します。
  # See: https://docs.docker.com/reference/compose-file/build/
  # ただし、 label の指定だけは build と同レベルに存在する labels で指定を行ってください。
  build = jsonencode({
    context    = "."
    dockerfile = "Dockerfile.app"
    additional_contexts = {
      resources = "../resources"
    }
    args = [
      "ENV=dev",
      "GIT_COMMIT",
    ]
  })

  # イメージに設定するラベルを指定してください。
  # これを再ビルドの条件として利用できます。
  # イメージがこのリソースの管理外で更新された場合に変更を検知するための手段として利用できます。
  labels = {
    label1 = "value1"
    label2 = "value2"
  }

  # イメージの再ビルドを行う条件の設定に利用できます。
  # 前回のこのリソースの作成・更新以降に、 Terraform 上の条件でイメージを再ビルドさせるのに利用できます。
  triggers = {
    sourcehash = data.archive.app.output_base64sha256
  }

  # イメージの更新時や削除時にイメージの削除を行うか。
  # デフォルトは false です。
  delete_image = false
}

output "sha256_digest" {
  value = containerregistry_compose.app.sha256_digest
}
```

## 認証


レジストリーへのイメージプッシュには通常、認証が必要です。
認証情報はプロバイダー設定で指定します。
各クラウドプロバイダーが提供する Terraform プロバイダーでは、
ephemeral リソースを利用して認証情報を取得することができます。

* Google Cloud (Artifact Registry): [`google_client_config` ephemeral](https://registry.terraform.io/providers/hashicorp/google/latest/docs/ephemeral-resources/client_config) の `access_token` を `password` に渡し、`username` は `oauth2accesstoken` にします。
    * google provider v7.10.0 以降が必要
    * https://cloud.google.com/artifact-registry/docs/docker/authentication#token も参照のこと。
* AWS ECR: [`aws_ecr_authorization_token` ephemeral](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/ephemeral-resources/ecr_authorization_token) の `user_name` / `password` をそのまま渡せます。
    * aws provider v6.22.0 以降が必要

設定例:

* Google Cloud (Artifact Registry)

    ```hcl
    ephemeral "google_client_config" "current" {}

    provider "containerregistry" {
      registry_auth = {
        "${ephemeral.google_client_config.current.region}-docker.pkg.dev" = {
          username = "oauth2accesstoken"
          password = ephemeral.google_client_config.current.access_token
        }  
      }
    }
    ```

* AWS ECR

    ```hcl
    ephemeral "aws_ecr_authorization_token" "current" {}

    provider "containerregistry" {
      registry_auth = {
        # proxy_endpoint には https:// が含まれているため、削除してからキーとして利用する。
        trimprefix(ephemeral.aws_ecr_authorization_token.current.proxy_endpoint, "https://") = {
          username = ephemeral.aws_ecr_authorization_token.current.user_name
          password = ephemeral.aws_ecr_authorization_token.current.password
        }
      }
    }
    ```

## 処理の概要

Terraform plugin framework を使用して実装しています: https://developer.hashicorp.com/terraform/plugin/framework

* リソース ID は UUID で作成します。
    * イメージタグの変更を想定しているため、イメージ URI を ID として使用できないため。

おおよそ以下の動作になります:

* Read()
    * 指定のコンテナーレジストリー/リポジトリーからイメージの情報を取得して反映。
    * このときのイメージ URI には、ステートファイルに保存されている URI で取得する。
        * イメージタグの変更時にも一旦古いイメージ情報を取得することになる。
    * このときの取得は Registry API を使用して docker pull は行わない。
* Create() / Update()
    * イメージを作成して push します。
    * イメージの作成処理は docker compose をライブラリーとして使用します。
* Delete()
    * delete_image が指定されている場合、イメージの削除を行います。
* Import()
    * インポートの ID としてはイメージ URI を指定する。
    * 実際にはイメージ URI をリソースの ID としては使用せず、ID を UUID から新規作成、および URI からイメージ情報を取り込む。


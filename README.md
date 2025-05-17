# containerregistry Terraform プロバイダー

## 概要

containerregistry Terraform プロバイダーは、
Container Registry (Docker Registry) 上のイメージの作成を行います。
以下の場合にイメージをビルドして push します:

* 指定されたイメージが存在しない場合
* イメージのラベルに変更がある場合
* その他、トリガー条件に変更があった場合

**ビルド指定に変更があっても再ビルドは行わないので注意してください**

## containerregistry_image リソース

```hcl
terraform {
  required_providers {
    aws = {
      source  = "containerregistry.tf.ikedam.jp/tf/containerregistry"
    }
  }
}

resource "containerregistry_image" "app" {
  # 作成および push するイメージ URI を指定します。
  # タグ部分を変数にすることで、タグの変更時にイメージを再作成するなどの動作に指定できます。
  image_uri = "your.image.registry/repository:v0.0.0"

  # build には、 docker compose v2 互換のビルド指定を記述します。
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

  auth = {
    # AWS ECR に対する認証
    aws_ecr = {
      profile = "ecr_auth"
    }

    # Google Cloud Artifact Registry に対する認証
    google_artifact_registry = {}

    # ユーザー名/パスワードによる認証
    username_password = {
      username = "..."
      password = "..."

      # AWS Secrets Manager / Google Cloud Secret Manager に保存された
      # username:password 形式のデータを使うこともできます。
      aws_secrets_manager   = "ARN"
      google_secret_manager = "projects/(PROJECT)/secrets/(SECRETNAME)/versions/(VERSION)"
    }
  }
}

output "sha256_digest" {
  value = containerregistry_image.app.sha256_digest
}
```

## 認証について

認証については以下をサポートしています:

* ユーザー名、パスワードによる認証
* ECR レジストリーの認証
* Google Cloud の Artifact Registry の認証

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


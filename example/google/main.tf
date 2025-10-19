terraform {
  required_version = ">= 1.10.6"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.7.0"
    }
    containerregistry = {
      source  = "tf-containerregistry.ikedam.jp/ikedam/containerregistry"
      version = "~> 0.4.0"
    }
    archive = {
      source  = "hashicorp/archive"
      version = "~> 2.7.1"
    }
  }
}

provider "google" {
  project = var.google_project
  region  = var.google_region
}

resource "google_project_service" "artifactregistry" {
  service = "artifactregistry.googleapis.com"

  disable_on_destroy = false
}

resource "google_artifact_registry_repository" "app" {
  repository_id = replace(var.basename, "_", "-")
  format        = "DOCKER"

  cleanup_policies {
    id     = "delete-untagged-images-after-1-days"
    action = "DELETE"
    condition {
      tag_state  = "UNTAGGED"
      older_than = "1d"
    }
  }

  depends_on = [
    google_project_service.artifactregistry,
  ]
}

data "archive_file" "app" {
  type        = "zip"
  source_dir  = "${path.module}/app"
  output_path = "${path.module}/app.zip"
}

resource "containerregistry_image" "app" {
  # 作成および push するイメージ URI を指定します。
  # タグ部分を変数にすることで、タグの変更時にイメージを再作成するなどの動作に指定できます。
  image_uri = "${google_artifact_registry_repository.app.registry_uri}/app:latest"

  # build には、 docker compose v2 互換のビルド指定を記述します。
  # See: https://docs.docker.com/reference/compose-file/build/
  # ただし、 label の指定だけは build と同レベルに存在する labels で指定を行ってください。
  build = jsonencode({
    context    = "${path.module}/app"
    dockerfile = "Dockerfile"
  })

  # イメージに設定するラベルを指定してください。
  # これを再ビルドの条件として利用できます。
  # イメージがこのリソースの管理外で更新された場合に変更を検知するための手段として利用できます。
  labels = {
    sourcehash = data.archive_file.app.output_sha512
  }

  # イメージの再ビルドを行う条件の設定に利用できます。
  # 前回のこのリソースの作成・更新以降に、 Terraform 上の条件でイメージを再ビルドさせるのに利用できます。
  triggers = {
    sourcehash = data.archive_file.app.output_md5
  }

  # イメージの更新時や削除時にイメージの削除を行うか。
  # デフォルトは false です。
  delete_image = false

  auth = {
    # Google Cloud Artifact Registry に対する認証
    google_artifact_registry = {}
  }
}

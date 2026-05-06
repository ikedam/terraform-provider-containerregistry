terraform {
  required_version = ">= 1.10.5"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "6.22.0"
    }
    archive = {
      source  = "hashicorp/archive"
      version = "2.7.1"
    }
    containerregistry = {
      source = "tf-containerregistry.ikedam.jp/ikedam/containerregistry"
    }
  }
}

provider "aws" {
  allowed_account_ids = var.aws_account_id_list
  region              = var.aws_region
}

ephemeral "aws_ecr_authorization_token" "current" {}

provider "containerregistry" {
  buildx_install_if_missing = true

  registry_auth = {
    trimprefix(ephemeral.aws_ecr_authorization_token.current.proxy_endpoint, "https://") = {
      username = ephemeral.aws_ecr_authorization_token.current.user_name
      password = ephemeral.aws_ecr_authorization_token.current.password
    }
  }
}

resource "aws_ecr_repository" "app" {
  name = var.basename
}

resource "aws_ecr_lifecycle_policy" "app" {
  repository = aws_ecr_repository.app.name
  policy     = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Delete untagged images"
        selection = {
          tagStatus   = "untagged"
          countType   = "sinceImagePushed"
          countNumber = 1
          countUnit   = "days"
        }
        action = {
          type = "expire"
        }
      },
    ]
  })
}

data "archive_file" "app" {
  type        = "zip"
  source_dir  = "${path.module}/../app"
  output_path = "${path.module}/../app.zip"
}

resource "containerregistry_compose" "app" {
  # 作成および push するイメージ URI を指定します。
  # タグ部分を変数にすることで、タグの変更時にイメージを再作成するなどの動作に指定できます。
  image_uri = "${aws_ecr_repository.app.repository_url}:latest"

  # build には、 docker compose v5 互換のビルド指定を記述します。
  # See: https://docs.docker.com/reference/compose-file/build/
  # ただし、 label の指定だけは build と同レベルに存在する labels で指定を行ってください。
  build = jsonencode({
    context    = "${path.module}/../app"
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
}

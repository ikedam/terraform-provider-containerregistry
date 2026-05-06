variable "basename" {
  type        = string
  description = "構築するリソースの共通prefix"
  default     = "crlocaltest"
}

variable "aws_account_id_list" {
  type        = list(string)
  description = "AWS Account ID List"
}

variable "aws_region" {
  type        = string
  description = "AWS Region"
}

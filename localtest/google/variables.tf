variable "basename" {
  type        = string
  description = "構築するリソースの共通prefix"
  default     = "crlocaltest"
}

variable "google_project" {
  type        = string
  description = "Google Cloud Project ID"
}

variable "google_region" {
  type        = string
  description = "Google Cloud Region"
}

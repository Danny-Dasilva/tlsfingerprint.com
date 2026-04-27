variable "project" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "zone" {
  description = "GCP zone"
  type        = string
  default     = "us-central1-a"
}

variable "name" {
  description = "Instance and resource name prefix"
  type        = string
  default     = "tlsfingerprint"
}

variable "machine_type" {
  description = "GCE machine type"
  type        = string
  default     = "e2-micro"
}

variable "cos_image" {
  description = "Container-Optimized OS image family"
  type        = string
  default     = "projects/cos-cloud/global/images/family/cos-stable"
}

variable "disk_size_gb" {
  description = "Boot disk size in GB"
  type        = number
  default     = 10
}

variable "service_account_email" {
  description = "Service account for the instance. Defaults to the project's compute default."
  type        = string
  default     = null
}

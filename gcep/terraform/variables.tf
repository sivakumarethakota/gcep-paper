variable "region" {
  description = "AWS region for the testbed."
  type        = string
  default     = "us-east-1"
}

variable "key_name" {
  description = "Existing EC2 key pair name for SSH. Create with `aws ec2 create-key-pair` before apply."
  type        = string
}

variable "allowed_ssh_cidr" {
  description = "CIDR allowed to SSH into the testbed. Lock this down; do NOT leave it as 0.0.0.0/0."
  type        = string
}

variable "resource_scale" {
  description = "Either 'dev' (t3.medium everywhere, ~$400/mo) or 'bench' (c6i.* as in the paper, ~$2300/mo)."
  type        = string
  default     = "dev"

  validation {
    condition     = contains(["dev", "bench"], var.resource_scale)
    error_message = "resource_scale must be 'dev' or 'bench'."
  }
}

variable "peer_count" {
  description = "Fabric peer nodes. Paper uses 4."
  type        = number
  default     = 4
}

variable "orderer_count" {
  description = "RAFT orderer nodes. Must be odd; paper uses 3."
  type        = number
  default     = 3
}

variable "committee_count" {
  description = "Threshold committee members (n). Paper uses 5 with t=3."
  type        = number
  default     = 5
}

variable "ipfs_count" {
  description = "IPFS-Cluster storage nodes. Paper uses 3."
  type        = number
  default     = 3
}

variable "project_tag" {
  description = "Tag applied to every resource for cost tracking."
  type        = string
  default     = "gcep"
}

locals {
  instance_types = {
    dev = {
      peer      = "t3.medium"
      orderer   = "t3.small"
      committee = "t3.small"
      ipfs      = "t3.small"
      loadgen   = "t3.large"
      monitor   = "t3.small"
    }
    bench = {
      peer      = "c6i.2xlarge"
      orderer   = "c6i.large"
      committee = "c6i.large"
      ipfs      = "c6i.large"
      loadgen   = "c6i.4xlarge"
      monitor   = "t3.medium"
    }
  }

  it = local.instance_types[var.resource_scale]

  common_tags = {
    Project   = var.project_tag
    ManagedBy = "terraform"
    Scale     = var.resource_scale
  }
}

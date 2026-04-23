terraform {
  required_version = ">= 1.6"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
  default_tags {
    tags = local.common_tags
  }
}

# ---------------------------------------------------------------------------
# AMI — Ubuntu 24.04 LTS, owned by Canonical
# ---------------------------------------------------------------------------
data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"]

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
  }
}

# ---------------------------------------------------------------------------
# VPC + networking
# ---------------------------------------------------------------------------
resource "aws_vpc" "main" {
  cidr_block           = "10.42.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = { Name = "${var.project_tag}-vpc" }
}

data "aws_availability_zones" "available" {
  state = "available"
}

resource "aws_subnet" "public" {
  count             = 2
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrsubnet(aws_vpc.main.cidr_block, 8, count.index)
  availability_zone = data.aws_availability_zones.available.names[count.index]

  map_public_ip_on_launch = true

  tags = { Name = "${var.project_tag}-public-${count.index}" }
}

resource "aws_internet_gateway" "gw" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = "${var.project_tag}-igw" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.gw.id
  }

  tags = { Name = "${var.project_tag}-rt-public" }
}

resource "aws_route_table_association" "public" {
  count          = 2
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# ---------------------------------------------------------------------------
# Security groups
# ---------------------------------------------------------------------------
resource "aws_security_group" "gcep" {
  name        = "${var.project_tag}-sg"
  description = "GCEP testbed SG — SSH from allowed CIDR, all else intra-VPC."
  vpc_id      = aws_vpc.main.id

  # SSH from operator
  ingress {
    description = "SSH"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.allowed_ssh_cidr]
  }

  # Prometheus / Grafana UI from operator (9090, 3000)
  ingress {
    description = "Prometheus"
    from_port   = 9090
    to_port     = 9090
    protocol    = "tcp"
    cidr_blocks = [var.allowed_ssh_cidr]
  }
  ingress {
    description = "Grafana"
    from_port   = 3000
    to_port     = 3000
    protocol    = "tcp"
    cidr_blocks = [var.allowed_ssh_cidr]
  }

  # Intra-VPC traffic — Fabric peer 7051, orderer 7050, gossip 7052,
  # IPFS swarm 4001, IPFS API 5001, cluster 9094-9096, node-exporter 9100.
  ingress {
    description = "Intra-VPC all-TCP"
    from_port   = 0
    to_port     = 65535
    protocol    = "tcp"
    cidr_blocks = [aws_vpc.main.cidr_block]
  }
  ingress {
    description = "Intra-VPC all-UDP"
    from_port   = 0
    to_port     = 65535
    protocol    = "udp"
    cidr_blocks = [aws_vpc.main.cidr_block]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# ---------------------------------------------------------------------------
# Cloud-init: install Docker + basics on every node. Role-specific provisioning
# is done later via Ansible against the IPs this outputs.
# ---------------------------------------------------------------------------
locals {
  user_data = <<-EOT
    #!/bin/bash
    set -euo pipefail
    apt-get update
    apt-get install -y docker.io docker-compose-plugin jq curl unzip
    systemctl enable --now docker
    usermod -aG docker ubuntu
    # node_exporter for Prometheus scraping
    cd /opt
    curl -sLO https://github.com/prometheus/node_exporter/releases/download/v1.8.2/node_exporter-1.8.2.linux-amd64.tar.gz
    tar xzf node_exporter-1.8.2.linux-amd64.tar.gz
    mv node_exporter-1.8.2.linux-amd64/node_exporter /usr/local/bin/
    cat >/etc/systemd/system/node_exporter.service <<UNIT
    [Unit]
    Description=Node Exporter
    After=network.target
    [Service]
    ExecStart=/usr/local/bin/node_exporter
    User=nobody
    [Install]
    WantedBy=multi-user.target
    UNIT
    systemctl enable --now node_exporter
  EOT
}

# ---------------------------------------------------------------------------
# EC2 roles — peers, orderers, committee, ipfs, loadgen, monitor
# ---------------------------------------------------------------------------
resource "aws_instance" "peer" {
  count                       = var.peer_count
  ami                         = data.aws_ami.ubuntu.id
  instance_type               = local.it.peer
  subnet_id                   = aws_subnet.public[count.index % 2].id
  vpc_security_group_ids      = [aws_security_group.gcep.id]
  key_name                    = var.key_name
  user_data                   = local.user_data
  associate_public_ip_address = true

  root_block_device {
    volume_size = 50
    volume_type = "gp3"
  }

  tags = { Name = "${var.project_tag}-peer-${count.index}", Role = "peer" }
}

resource "aws_instance" "orderer" {
  count                       = var.orderer_count
  ami                         = data.aws_ami.ubuntu.id
  instance_type               = local.it.orderer
  subnet_id                   = aws_subnet.public[count.index % 2].id
  vpc_security_group_ids      = [aws_security_group.gcep.id]
  key_name                    = var.key_name
  user_data                   = local.user_data
  associate_public_ip_address = true

  root_block_device {
    volume_size = 30
    volume_type = "gp3"
  }

  tags = { Name = "${var.project_tag}-orderer-${count.index}", Role = "orderer" }
}

resource "aws_instance" "committee" {
  count                       = var.committee_count
  ami                         = data.aws_ami.ubuntu.id
  instance_type               = local.it.committee
  subnet_id                   = aws_subnet.public[count.index % 2].id
  vpc_security_group_ids      = [aws_security_group.gcep.id]
  key_name                    = var.key_name
  user_data                   = local.user_data
  associate_public_ip_address = true

  root_block_device {
    volume_size = 20
    volume_type = "gp3"
  }

  tags = { Name = "${var.project_tag}-committee-${count.index}", Role = "committee" }
}

resource "aws_instance" "ipfs" {
  count                       = var.ipfs_count
  ami                         = data.aws_ami.ubuntu.id
  instance_type               = local.it.ipfs
  subnet_id                   = aws_subnet.public[count.index % 2].id
  vpc_security_group_ids      = [aws_security_group.gcep.id]
  key_name                    = var.key_name
  user_data                   = local.user_data
  associate_public_ip_address = true

  root_block_device {
    volume_size = 100
    volume_type = "gp3"
  }

  tags = { Name = "${var.project_tag}-ipfs-${count.index}", Role = "ipfs" }
}

resource "aws_instance" "loadgen" {
  ami                         = data.aws_ami.ubuntu.id
  instance_type               = local.it.loadgen
  subnet_id                   = aws_subnet.public[0].id
  vpc_security_group_ids      = [aws_security_group.gcep.id]
  key_name                    = var.key_name
  user_data                   = local.user_data
  associate_public_ip_address = true

  root_block_device {
    volume_size = 50
    volume_type = "gp3"
  }

  tags = { Name = "${var.project_tag}-loadgen", Role = "loadgen" }
}

resource "aws_instance" "monitor" {
  ami                         = data.aws_ami.ubuntu.id
  instance_type               = local.it.monitor
  subnet_id                   = aws_subnet.public[0].id
  vpc_security_group_ids      = [aws_security_group.gcep.id]
  key_name                    = var.key_name
  user_data                   = local.user_data
  associate_public_ip_address = true

  root_block_device {
    volume_size = 50
    volume_type = "gp3"
  }

  tags = { Name = "${var.project_tag}-monitor", Role = "monitor" }
}

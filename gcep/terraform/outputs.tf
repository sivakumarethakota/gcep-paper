output "peer_public_ips" {
  description = "Fabric peer public IPs (SSH target)."
  value       = aws_instance.peer[*].public_ip
}

output "peer_private_ips" {
  description = "Fabric peer private IPs (intra-VPC Fabric traffic)."
  value       = aws_instance.peer[*].private_ip
}

output "orderer_public_ips" {
  description = "RAFT orderer public IPs."
  value       = aws_instance.orderer[*].public_ip
}

output "orderer_private_ips" {
  description = "RAFT orderer private IPs."
  value       = aws_instance.orderer[*].private_ip
}

output "committee_public_ips" {
  description = "Committee member public IPs."
  value       = aws_instance.committee[*].public_ip
}

output "ipfs_public_ips" {
  description = "IPFS-Cluster node public IPs."
  value       = aws_instance.ipfs[*].public_ip
}

output "loadgen_public_ip" {
  description = "Caliper load generator IP."
  value       = aws_instance.loadgen.public_ip
}

output "monitor_public_ip" {
  description = "Prometheus + Grafana IP. Grafana on :3000, Prometheus on :9090."
  value       = aws_instance.monitor.public_ip
}

output "ansible_inventory" {
  description = "Paste-ready Ansible inventory in INI format. Redirect to a file: `terraform output -raw ansible_inventory > inventory.ini`."
  value = join("\n", concat(
    ["[peers]"],
    [for i, ip in aws_instance.peer[*].public_ip : "peer${i} ansible_host=${ip} ansible_user=ubuntu private_ip=${aws_instance.peer[i].private_ip}"],
    ["", "[orderers]"],
    [for i, ip in aws_instance.orderer[*].public_ip : "orderer${i} ansible_host=${ip} ansible_user=ubuntu private_ip=${aws_instance.orderer[i].private_ip}"],
    ["", "[committee]"],
    [for i, ip in aws_instance.committee[*].public_ip : "committee${i} ansible_host=${ip} ansible_user=ubuntu private_ip=${aws_instance.committee[i].private_ip}"],
    ["", "[ipfs]"],
    [for i, ip in aws_instance.ipfs[*].public_ip : "ipfs${i} ansible_host=${ip} ansible_user=ubuntu private_ip=${aws_instance.ipfs[i].private_ip}"],
    ["", "[loadgen]", "loadgen ansible_host=${aws_instance.loadgen.public_ip} ansible_user=ubuntu"],
    ["", "[monitor]", "monitor ansible_host=${aws_instance.monitor.public_ip} ansible_user=ubuntu"],
  ))
}

# cluster information

output "cluster_name" {
  description = "name of the deployed okd cluster"
  value       = module.okd_cluster.cluster_name
}

# cluster summary

output "cluster_summary" {
  description = "summary of the deployed cluster"
  value = {
    cluster_name = module.okd_cluster.cluster_name
    total_nodes  = module.okd_cluster.cluster_nodes_total
    bootstrap    = module.okd_cluster.bootstrap_vm_name
    masters      = module.okd_cluster.master_vm_names
    workers      = module.okd_cluster.worker_vm_names
  }
}

# vm ids

output "vm_ids" {
  description = "all vm ids for reference"
  value = {
    bootstrap = module.okd_cluster.bootstrap_vm_id
    masters   = module.okd_cluster.master_vm_ids
    workers   = module.okd_cluster.worker_vm_ids
  }
}

# cluster resources

output "cluster_resources" {
  description = "total cluster resources"
  value       = module.okd_cluster.cluster_resources
}

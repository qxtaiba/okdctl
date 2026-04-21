// tflint config for the okdctl terraform tree.
//
// The `terraform` plugin's `recommended` preset turns on the lint rules
// that catch the most common HCL mistakes — required-providers,
// required-version, unused-declarations, naming conventions, and
// module-source pinning.

plugin "terraform" {
  enabled = true
  preset  = "recommended"
}

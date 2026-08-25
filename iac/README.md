# IaC modules (scaffold)

Real infrastructure-as-code lives here in production:

- `azure/` - Bicep for Azure Container Apps + Key Vault
- `aws/` - Terraform for ECS/Fargate + Secrets Manager
- `helm/` - chart for local Kubernetes

The `deploykit` CLI renders and applies these with secure-by-default settings.

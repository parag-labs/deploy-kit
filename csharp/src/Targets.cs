using System;
using System.Collections.Generic;
using System.Linq;
using System.Text;

namespace DeployKit;

/// <summary>Renders a deployment <see cref="Plan"/> for a specific environment.</summary>
public interface ITarget
{
    /// <summary>The target's canonical name.</summary>
    string Name { get; }

    /// <summary>Renders the deployment plan for <paramref name="cfg"/>.</summary>
    Plan CreatePlan(Config cfg);
}

internal sealed class LocalTarget : ITarget
{
    public string Name => "local";

    public Plan CreatePlan(Config cfg)
    {
        var secretBackend = cfg.SecretBackend == "auto" ? "k8s-secret" : cfg.SecretBackend;
        var steps = new List<string>
        {
            "ensure kind/minikube cluster is running",
            $"create namespace {cfg.AppName}",
            $"create {secretBackend} for app credentials",
            $"helm install {cfg.AppName} ./charts/{cfg.AppName} --set image={cfg.Image}",
        };
        if (cfg.DenyAllEgress)
        {
            steps.Add("apply default-deny NetworkPolicy");
        }

        var manifests = new Dictionary<string, string>
        {
            ["values.yaml"] = Deploy.HelmValues(cfg, secretBackend),
        };
        return new Plan("local", steps, manifests, Deploy.SecurityChecks(cfg));
    }
}

internal sealed class AzureTarget : ITarget
{
    public string Name => "azure";

    public Plan CreatePlan(Config cfg)
    {
        var secretBackend = cfg.SecretBackend == "auto" ? "keyvault" : cfg.SecretBackend;
        var ingress = cfg.Tls ? "bind managed identity; enable ingress with TLS" : "enable ingress";
        var steps = new List<string>
        {
            "az group create + provision Azure Container Apps environment (Bicep)",
            $"store secrets in Azure {secretBackend}",
            $"deploy {cfg.AppName} container app (image={cfg.Image}, replicas={cfg.Replicas})",
            ingress,
        };
        if (cfg.DenyAllEgress)
        {
            steps.Add("apply egress restrictions via NSG / container app policy");
        }

        var manifests = new Dictionary<string, string>
        {
            ["main.bicep"] = Deploy.Bicep(cfg, secretBackend),
        };
        return new Plan("azure", steps, manifests, Deploy.SecurityChecks(cfg));
    }
}

internal sealed class AwsTarget : ITarget
{
    public string Name => "aws";

    public Plan CreatePlan(Config cfg)
    {
        var secretBackend = cfg.SecretBackend == "auto" ? "secrets-manager" : cfg.SecretBackend;
        var front = cfg.Tls ? "front with ALB + ACM TLS cert" : "front with ALB";
        var steps = new List<string>
        {
            "terraform apply: VPC + ECS/Fargate cluster",
            $"store secrets in AWS {secretBackend}",
            $"deploy {cfg.AppName} service (image={cfg.Image}, desired_count={cfg.Replicas})",
            front,
        };
        if (cfg.DenyAllEgress)
        {
            steps.Add("apply restrictive security groups (deny-all egress baseline)");
        }

        var manifests = new Dictionary<string, string>
        {
            ["main.tf"] = Deploy.Terraform(cfg, secretBackend),
        };
        return new Plan("aws", steps, manifests, Deploy.SecurityChecks(cfg));
    }
}

/// <summary>
/// Target registry plus the secure-by-default checks and plan rendering. This is
/// the deterministic core ported from the Python <c>deploykit</c> package;
/// the argparse CLI and stubbed cloud apply/destroy calls are not ported.
/// </summary>
public static class Deploy
{
    /// <summary>The canonical target names, in registration order.</summary>
    public static IReadOnlyList<string> TargetNames { get; } = new[] { "local", "azure", "aws" };

    private static readonly IReadOnlyDictionary<string, ITarget> Registry =
        new Dictionary<string, ITarget>
        {
            ["local"] = new LocalTarget(),
            ["azure"] = new AzureTarget(),
            ["aws"] = new AwsTarget(),
        };

    /// <summary>
    /// Resolves a target by name. Throws <see cref="ArgumentException"/> with a
    /// message matching the Python <c>ValueError</c> for unknown names.
    /// </summary>
    public static ITarget GetTarget(string name)
    {
        if (Registry.TryGetValue(name, out var target))
        {
            return target;
        }

        var available = "[" + string.Join(", ", TargetNames.Select(n => $"'{n}'")) + "]";
        throw new ArgumentException($"unknown target '{name}'. available: {available}");
    }

    /// <summary>
    /// Renders the human-readable plan for <paramref name="targetName"/> and
    /// returns the text with the intended exit code (1 when secure-by-default
    /// warnings fire, otherwise 0). Mirrors the Python CLI's <c>_print_plan</c>.
    /// </summary>
    public static (string Text, int Code) RenderPlan(string targetName, Config cfg)
    {
        var plan = GetTarget(targetName).CreatePlan(cfg);
        var sb = new StringBuilder();
        sb.Append($"== DeployKit plan: {plan.Target} / {cfg.AppName} ==\n");
        for (var i = 0; i < plan.Steps.Count; i++)
        {
            sb.Append($"  {i + 1}. {plan.Steps[i]}\n");
        }

        if (plan.Warnings.Count > 0)
        {
            sb.Append("\n  SECURITY WARNINGS:\n");
            foreach (var w in plan.Warnings)
            {
                sb.Append($"    ! {w}\n");
            }

            return (sb.ToString(), 1);
        }

        sb.Append("\n  secure-by-default checks: PASS\n");
        return (sb.ToString(), 0);
    }

    internal static List<string> SecurityChecks(Config cfg)
    {
        var warnings = new List<string>();
        if (!cfg.Tls)
        {
            warnings.Add("TLS disabled -- traffic would be unencrypted");
        }

        if (!cfg.DenyAllEgress)
        {
            warnings.Add("egress not restricted -- data exfil risk");
        }

        if (cfg.SecretBackend == "plaintext")
        {
            warnings.Add("secrets stored in plaintext");
        }

        return warnings;
    }

    internal static string HelmValues(Config cfg, string secretBackend) =>
        $"image: {cfg.Image}\nreplicas: {cfg.Replicas}\ntls: {Lower(cfg.Tls)}\n"
        + $"secretBackend: {secretBackend}\nnetworkPolicy:\n  denyAllEgress: {Lower(cfg.DenyAllEgress)}\n";

    internal static string Bicep(Config cfg, string secretBackend) =>
        $"// Bicep for {cfg.AppName}\nparam image string = '{cfg.Image}'\n"
        + $"param secretBackend string = '{secretBackend}'\n";

    internal static string Terraform(Config cfg, string secretBackend) =>
        $"# Terraform for {cfg.AppName}\nvariable \"image\" {{ default = \"{cfg.Image}\" }}\n"
        + $"# secrets: {secretBackend}\n";

    private static string Lower(bool value) => value ? "true" : "false";
}

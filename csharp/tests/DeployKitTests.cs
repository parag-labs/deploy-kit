using System;
using System.Collections.Generic;
using System.Linq;
using DeployKit;
using Xunit;

namespace DeployKit.Tests;

public class DeployKitTests
{
    private static Config Cfg() => new Config();

    [Fact]
    public void DefaultConfigMatchesPython()
    {
        var c = new Config();
        Assert.Equal("ledgerrag", c.AppName);
        Assert.Equal("ghcr.io/example/ledgerrag:latest", c.Image);
        Assert.Equal(2, c.Replicas);
        Assert.True(c.Tls);
        Assert.Equal("auto", c.SecretBackend);
        Assert.True(c.DenyAllEgress);
    }

    [Theory]
    [InlineData("local")]
    [InlineData("azure")]
    [InlineData("aws")]
    public void AllTargetsProducePlans(string name)
    {
        var plan = Deploy.GetTarget(name).CreatePlan(Cfg());
        Assert.NotEmpty(plan.Steps);
        Assert.NotEmpty(plan.Manifests);
        Assert.True(plan.IsSecure());
        Assert.Equal(name, plan.Target);
    }

    [Fact]
    public void SecureDefaultsPass()
    {
        var plan = Deploy.GetTarget("azure").CreatePlan(Cfg());
        Assert.True(plan.IsSecure());
        Assert.Empty(plan.Warnings);
    }

    [Fact]
    public void InsecureConfigFlagged()
    {
        var cfg = Cfg();
        cfg.Tls = false;
        cfg.DenyAllEgress = false;
        var plan = Deploy.GetTarget("aws").CreatePlan(cfg);
        Assert.False(plan.IsSecure());
        Assert.Contains(plan.Warnings, w => w.Contains("TLS"));
        Assert.Contains(plan.Warnings, w => w.Contains("egress"));
    }

    [Fact]
    public void PlaintextSecretBackendFlagged()
    {
        var cfg = Cfg();
        cfg.SecretBackend = "plaintext";
        var plan = Deploy.GetTarget("local").CreatePlan(cfg);
        Assert.False(plan.IsSecure());
        Assert.Single(plan.Warnings);
        Assert.Contains("plaintext", plan.Warnings[0]);
    }

    [Fact]
    public void WarningOrderingIsStable()
    {
        var cfg = Cfg();
        cfg.Tls = false;
        cfg.DenyAllEgress = false;
        cfg.SecretBackend = "plaintext";
        var plan = Deploy.GetTarget("local").CreatePlan(cfg);
        Assert.Equal(
            new[]
            {
                "TLS disabled -- traffic would be unencrypted",
                "egress not restricted -- data exfil risk",
                "secrets stored in plaintext",
            },
            plan.Warnings);
    }

    [Fact]
    public void TargetSelectsRightSecretBackend()
    {
        Assert.Contains("keyvault", Deploy.GetTarget("azure").CreatePlan(Cfg()).Manifests["main.bicep"]);
        Assert.Contains(
            "secrets-manager",
            string.Join("\n", Deploy.GetTarget("aws").CreatePlan(Cfg()).Steps));
        Assert.Contains("k8s-secret", Deploy.GetTarget("local").CreatePlan(Cfg()).Manifests["values.yaml"]);
    }

    [Fact]
    public void NonAutoSecretBackendOverride()
    {
        var cfg = Cfg();
        cfg.SecretBackend = "vault";
        var plan = Deploy.GetTarget("local").CreatePlan(cfg);
        Assert.Contains("secretBackend: vault", plan.Manifests["values.yaml"]);
    }

    [Fact]
    public void UnknownTargetThrowsWithExactMessage()
    {
        var ex = Assert.Throws<ArgumentException>(() => Deploy.GetTarget("gcp"));
        Assert.Equal("unknown target 'gcp'. available: ['local', 'azure', 'aws']", ex.Message);
    }

    [Fact]
    public void HelmValuesExact()
    {
        var plan = Deploy.GetTarget("local").CreatePlan(Cfg());
        Assert.Equal(
            "image: ghcr.io/example/ledgerrag:latest\nreplicas: 2\ntls: true\nsecretBackend: k8s-secret\nnetworkPolicy:\n  denyAllEgress: true\n",
            plan.Manifests["values.yaml"]);
    }

    [Fact]
    public void BicepExact()
    {
        var plan = Deploy.GetTarget("azure").CreatePlan(Cfg());
        Assert.Equal(
            "// Bicep for ledgerrag\nparam image string = 'ghcr.io/example/ledgerrag:latest'\nparam secretBackend string = 'keyvault'\n",
            plan.Manifests["main.bicep"]);
    }

    [Fact]
    public void TerraformExact()
    {
        var plan = Deploy.GetTarget("aws").CreatePlan(Cfg());
        Assert.Equal(
            "# Terraform for ledgerrag\nvariable \"image\" { default = \"ghcr.io/example/ledgerrag:latest\" }\n# secrets: secrets-manager\n",
            plan.Manifests["main.tf"]);
    }

    [Fact]
    public void TlsOffChangesIngressAndFrontSteps()
    {
        var cfg = Cfg();
        cfg.Tls = false;
        var azureSteps = string.Join("\n", Deploy.GetTarget("azure").CreatePlan(cfg).Steps);
        Assert.DoesNotContain("with TLS", azureSteps);
        Assert.Contains("enable ingress", azureSteps);
        var awsSteps = string.Join("\n", Deploy.GetTarget("aws").CreatePlan(cfg).Steps);
        Assert.DoesNotContain("ACM TLS cert", awsSteps);
        Assert.Contains("front with ALB", awsSteps);
    }

    [Fact]
    public void DenyEgressOffRemovesStep()
    {
        var cfg = Cfg();
        cfg.DenyAllEgress = false;
        var steps = Deploy.GetTarget("local").CreatePlan(cfg).Steps;
        Assert.Equal(4, steps.Count);
        Assert.DoesNotContain(steps, s => s.Contains("NetworkPolicy"));
    }

    [Fact]
    public void LocalStepsExactWithDefaults()
    {
        var steps = Deploy.GetTarget("local").CreatePlan(Cfg()).Steps;
        Assert.Equal(
            new[]
            {
                "ensure kind/minikube cluster is running",
                "create namespace ledgerrag",
                "create k8s-secret for app credentials",
                "helm install ledgerrag ./charts/ledgerrag --set image=ghcr.io/example/ledgerrag:latest",
                "apply default-deny NetworkPolicy",
            },
            steps);
    }

    [Fact]
    public void RenderPlanSecureReturnsZero()
    {
        var (text, code) = Deploy.RenderPlan("local", Cfg());
        Assert.Equal(0, code);
        Assert.Contains("secure-by-default checks: PASS", text);
        Assert.StartsWith("== DeployKit plan: local / ledgerrag ==\n", text);
    }

    [Fact]
    public void RenderPlanInsecureReturnsOne()
    {
        var cfg = Cfg();
        cfg.Tls = false;
        cfg.DenyAllEgress = false;
        var (text, code) = Deploy.RenderPlan("aws", cfg);
        Assert.Equal(1, code);
        Assert.Contains("SECURITY WARNINGS:", text);
    }

    [Fact]
    public void RenderPlanUnknownTargetThrows()
    {
        Assert.Throws<ArgumentException>(() => Deploy.RenderPlan("gcp", Cfg()));
    }

    [Fact]
    public void TargetNamesOrder()
    {
        Assert.Equal(new[] { "local", "azure", "aws" }, Deploy.TargetNames.ToArray());
    }

    [Fact]
    public void TargetNameProperty()
    {
        Assert.Equal("local", Deploy.GetTarget("local").Name);
        Assert.Equal("azure", Deploy.GetTarget("azure").Name);
        Assert.Equal("aws", Deploy.GetTarget("aws").Name);
    }
}

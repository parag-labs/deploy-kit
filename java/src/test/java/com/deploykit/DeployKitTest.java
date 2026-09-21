package com.deploykit;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.List;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

class DeployKitTest {

    private static Config cfg() {
        return new Config();
    }

    @Test
    void defaultConfigMatchesPython() {
        Config c = new Config();
        assertEquals("ledgerrag", c.getAppName());
        assertEquals("ghcr.io/example/ledgerrag:latest", c.getImage());
        assertEquals(2, c.getReplicas());
        assertTrue(c.isTls());
        assertEquals("auto", c.getSecretBackend());
        assertTrue(c.isDenyAllEgress());
    }

    @ParameterizedTest
    @ValueSource(strings = {"local", "azure", "aws"})
    void allTargetsProducePlans(String name) {
        Plan plan = Deploy.getTarget(name).plan(cfg());
        assertFalse(plan.getSteps().isEmpty());
        assertFalse(plan.getManifests().isEmpty());
        assertTrue(plan.isSecure());
        assertEquals(name, plan.getTarget());
    }

    @Test
    void secureDefaultsPass() {
        Plan plan = Deploy.getTarget("azure").plan(cfg());
        assertTrue(plan.isSecure());
        assertTrue(plan.getWarnings().isEmpty());
    }

    @Test
    void insecureConfigFlagged() {
        Config c = cfg();
        c.setTls(false);
        c.setDenyAllEgress(false);
        Plan plan = Deploy.getTarget("aws").plan(c);
        assertFalse(plan.isSecure());
        assertTrue(plan.getWarnings().stream().anyMatch(w -> w.contains("TLS")));
        assertTrue(plan.getWarnings().stream().anyMatch(w -> w.contains("egress")));
    }

    @Test
    void plaintextSecretBackendFlagged() {
        Config c = cfg();
        c.setSecretBackend("plaintext");
        Plan plan = Deploy.getTarget("local").plan(c);
        assertFalse(plan.isSecure());
        assertEquals(1, plan.getWarnings().size());
        assertTrue(plan.getWarnings().get(0).contains("plaintext"));
    }

    @Test
    void warningOrderingIsStable() {
        Config c = cfg();
        c.setTls(false);
        c.setDenyAllEgress(false);
        c.setSecretBackend("plaintext");
        Plan plan = Deploy.getTarget("local").plan(c);
        assertEquals(
            List.of(
                "TLS disabled -- traffic would be unencrypted",
                "egress not restricted -- data exfil risk",
                "secrets stored in plaintext"),
            plan.getWarnings());
    }

    @Test
    void targetSelectsRightSecretBackend() {
        assertTrue(Deploy.getTarget("azure").plan(cfg()).getManifests().get("main.bicep").contains("keyvault"));
        assertTrue(String.join("\n", Deploy.getTarget("aws").plan(cfg()).getSteps()).contains("secrets-manager"));
        assertTrue(Deploy.getTarget("local").plan(cfg()).getManifests().get("values.yaml").contains("k8s-secret"));
    }

    @Test
    void nonAutoSecretBackendOverride() {
        Config c = cfg();
        c.setSecretBackend("vault");
        Plan plan = Deploy.getTarget("local").plan(c);
        assertTrue(plan.getManifests().get("values.yaml").contains("secretBackend: vault"));
    }

    @Test
    void unknownTargetThrowsWithExactMessage() {
        IllegalArgumentException ex =
            assertThrows(IllegalArgumentException.class, () -> Deploy.getTarget("gcp"));
        assertEquals("unknown target 'gcp'. available: ['local', 'azure', 'aws']", ex.getMessage());
    }

    @Test
    void helmValuesExact() {
        Plan plan = Deploy.getTarget("local").plan(cfg());
        assertEquals(
            "image: ghcr.io/example/ledgerrag:latest\nreplicas: 2\ntls: true\n"
                + "secretBackend: k8s-secret\nnetworkPolicy:\n  denyAllEgress: true\n",
            plan.getManifests().get("values.yaml"));
    }

    @Test
    void bicepExact() {
        Plan plan = Deploy.getTarget("azure").plan(cfg());
        assertEquals(
            "// Bicep for ledgerrag\nparam image string = 'ghcr.io/example/ledgerrag:latest'\n"
                + "param secretBackend string = 'keyvault'\n",
            plan.getManifests().get("main.bicep"));
    }

    @Test
    void terraformExact() {
        Plan plan = Deploy.getTarget("aws").plan(cfg());
        assertEquals(
            "# Terraform for ledgerrag\nvariable \"image\" { default = \"ghcr.io/example/ledgerrag:latest\" }\n"
                + "# secrets: secrets-manager\n",
            plan.getManifests().get("main.tf"));
    }

    @Test
    void tlsOffChangesIngressAndFrontSteps() {
        Config c = cfg();
        c.setTls(false);
        String azureSteps = String.join("\n", Deploy.getTarget("azure").plan(c).getSteps());
        assertFalse(azureSteps.contains("with TLS"));
        assertTrue(azureSteps.contains("enable ingress"));
        String awsSteps = String.join("\n", Deploy.getTarget("aws").plan(c).getSteps());
        assertFalse(awsSteps.contains("ACM TLS cert"));
        assertTrue(awsSteps.contains("front with ALB"));
    }

    @Test
    void denyEgressOffRemovesStep() {
        Config c = cfg();
        c.setDenyAllEgress(false);
        List<String> steps = Deploy.getTarget("local").plan(c).getSteps();
        assertEquals(4, steps.size());
        assertFalse(steps.stream().anyMatch(s -> s.contains("NetworkPolicy")));
    }

    @Test
    void localStepsExactWithDefaults() {
        List<String> steps = Deploy.getTarget("local").plan(cfg()).getSteps();
        assertEquals(
            List.of(
                "ensure kind/minikube cluster is running",
                "create namespace ledgerrag",
                "create k8s-secret for app credentials",
                "helm install ledgerrag ./charts/ledgerrag --set image=ghcr.io/example/ledgerrag:latest",
                "apply default-deny NetworkPolicy"),
            steps);
    }

    @Test
    void renderPlanSecureReturnsZero() {
        RenderResult result = Deploy.renderPlan("local", cfg());
        assertEquals(0, result.code());
        assertTrue(result.text().contains("secure-by-default checks: PASS"));
        assertTrue(result.text().startsWith("== DeployKit plan: local / ledgerrag ==\n"));
    }

    @Test
    void renderPlanInsecureReturnsOne() {
        Config c = cfg();
        c.setTls(false);
        c.setDenyAllEgress(false);
        RenderResult result = Deploy.renderPlan("aws", c);
        assertEquals(1, result.code());
        assertTrue(result.text().contains("SECURITY WARNINGS:"));
    }

    @Test
    void renderPlanUnknownTargetThrows() {
        assertThrows(IllegalArgumentException.class, () -> Deploy.renderPlan("gcp", cfg()));
    }

    @Test
    void targetNamesOrder() {
        assertEquals(List.of("local", "azure", "aws"), Deploy.TARGET_NAMES);
    }

    @Test
    void targetNameMethod() {
        assertEquals("local", Deploy.getTarget("local").name());
        assertEquals("azure", Deploy.getTarget("azure").name());
        assertEquals("aws", Deploy.getTarget("aws").name());
    }
}

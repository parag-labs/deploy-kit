package com.deploykit;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

final class AzureTarget implements Target {
    @Override
    public String name() {
        return "azure";
    }

    @Override
    public Plan plan(Config cfg) {
        String secretBackend = "auto".equals(cfg.getSecretBackend()) ? "keyvault" : cfg.getSecretBackend();
        String ingress = cfg.isTls() ? "bind managed identity; enable ingress with TLS" : "enable ingress";
        List<String> steps = new ArrayList<>();
        steps.add("az group create + provision Azure Container Apps environment (Bicep)");
        steps.add("store secrets in Azure " + secretBackend);
        steps.add("deploy " + cfg.getAppName() + " container app (image=" + cfg.getImage()
            + ", replicas=" + cfg.getReplicas() + ")");
        steps.add(ingress);
        if (cfg.isDenyAllEgress()) {
            steps.add("apply egress restrictions via NSG / container app policy");
        }
        Map<String, String> manifests = new LinkedHashMap<>();
        manifests.put("main.bicep", Deploy.bicep(cfg, secretBackend));
        return new Plan("azure", steps, manifests, Deploy.securityChecks(cfg));
    }
}

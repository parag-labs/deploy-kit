package com.deploykit;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

final class LocalTarget implements Target {
    @Override
    public String name() {
        return "local";
    }

    @Override
    public Plan plan(Config cfg) {
        String secretBackend = "auto".equals(cfg.getSecretBackend()) ? "k8s-secret" : cfg.getSecretBackend();
        List<String> steps = new ArrayList<>();
        steps.add("ensure kind/minikube cluster is running");
        steps.add("create namespace " + cfg.getAppName());
        steps.add("create " + secretBackend + " for app credentials");
        steps.add("helm install " + cfg.getAppName() + " ./charts/" + cfg.getAppName()
            + " --set image=" + cfg.getImage());
        if (cfg.isDenyAllEgress()) {
            steps.add("apply default-deny NetworkPolicy");
        }
        Map<String, String> manifests = new LinkedHashMap<>();
        manifests.put("values.yaml", Deploy.helmValues(cfg, secretBackend));
        return new Plan("local", steps, manifests, Deploy.securityChecks(cfg));
    }
}

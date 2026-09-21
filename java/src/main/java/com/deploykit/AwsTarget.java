package com.deploykit;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

final class AwsTarget implements Target {
    @Override
    public String name() {
        return "aws";
    }

    @Override
    public Plan plan(Config cfg) {
        String secretBackend = "auto".equals(cfg.getSecretBackend()) ? "secrets-manager" : cfg.getSecretBackend();
        String front = cfg.isTls() ? "front with ALB + ACM TLS cert" : "front with ALB";
        List<String> steps = new ArrayList<>();
        steps.add("terraform apply: VPC + ECS/Fargate cluster");
        steps.add("store secrets in AWS " + secretBackend);
        steps.add("deploy " + cfg.getAppName() + " service (image=" + cfg.getImage()
            + ", desired_count=" + cfg.getReplicas() + ")");
        steps.add(front);
        if (cfg.isDenyAllEgress()) {
            steps.add("apply restrictive security groups (deny-all egress baseline)");
        }
        Map<String, String> manifests = new LinkedHashMap<>();
        manifests.put("main.tf", Deploy.terraform(cfg, secretBackend));
        return new Plan("aws", steps, manifests, Deploy.securityChecks(cfg));
    }
}

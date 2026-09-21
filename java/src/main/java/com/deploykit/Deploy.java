package com.deploykit;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * Target registry plus the secure-by-default checks and plan rendering. This is
 * the deterministic core ported from the Python {@code deploykit} package; the
 * argparse CLI and stubbed cloud apply/destroy calls are not ported.
 */
public final class Deploy {
    /** The canonical target names, in registration order. */
    public static final List<String> TARGET_NAMES = List.of("local", "azure", "aws");

    private static final Map<String, Target> REGISTRY = new LinkedHashMap<>();

    static {
        REGISTRY.put("local", new LocalTarget());
        REGISTRY.put("azure", new AzureTarget());
        REGISTRY.put("aws", new AwsTarget());
    }

    private Deploy() {
    }

    /**
     * Resolves a target by name. Throws {@link IllegalArgumentException} with a
     * message matching the Python {@code ValueError} for unknown names.
     */
    public static Target getTarget(String name) {
        Target target = REGISTRY.get(name);
        if (target == null) {
            throw new IllegalArgumentException(
                "unknown target '" + name + "'. available: " + formatNameList(TARGET_NAMES));
        }
        return target;
    }

    /**
     * Renders the human-readable plan for {@code targetName} and returns the text
     * with the intended exit code (1 when secure-by-default warnings fire,
     * otherwise 0). Mirrors the Python CLI's {@code _print_plan}.
     */
    public static RenderResult renderPlan(String targetName, Config cfg) {
        Plan plan = getTarget(targetName).plan(cfg);
        StringBuilder sb = new StringBuilder();
        sb.append("== DeployKit plan: ").append(plan.getTarget()).append(" / ")
            .append(cfg.getAppName()).append(" ==\n");
        List<String> steps = plan.getSteps();
        for (int i = 0; i < steps.size(); i++) {
            sb.append("  ").append(i + 1).append(". ").append(steps.get(i)).append("\n");
        }
        if (!plan.getWarnings().isEmpty()) {
            sb.append("\n  SECURITY WARNINGS:\n");
            for (String w : plan.getWarnings()) {
                sb.append("    ! ").append(w).append("\n");
            }
            return new RenderResult(sb.toString(), 1);
        }
        sb.append("\n  secure-by-default checks: PASS\n");
        return new RenderResult(sb.toString(), 0);
    }

    static List<String> securityChecks(Config cfg) {
        List<String> warnings = new ArrayList<>();
        if (!cfg.isTls()) {
            warnings.add("TLS disabled -- traffic would be unencrypted");
        }
        if (!cfg.isDenyAllEgress()) {
            warnings.add("egress not restricted -- data exfil risk");
        }
        if ("plaintext".equals(cfg.getSecretBackend())) {
            warnings.add("secrets stored in plaintext");
        }
        return warnings;
    }

    static String helmValues(Config cfg, String secretBackend) {
        return "image: " + cfg.getImage() + "\nreplicas: " + cfg.getReplicas()
            + "\ntls: " + lower(cfg.isTls()) + "\nsecretBackend: " + secretBackend
            + "\nnetworkPolicy:\n  denyAllEgress: " + lower(cfg.isDenyAllEgress()) + "\n";
    }

    static String bicep(Config cfg, String secretBackend) {
        return "// Bicep for " + cfg.getAppName() + "\nparam image string = '" + cfg.getImage()
            + "'\nparam secretBackend string = '" + secretBackend + "'\n";
    }

    static String terraform(Config cfg, String secretBackend) {
        return "# Terraform for " + cfg.getAppName() + "\nvariable \"image\" { default = \""
            + cfg.getImage() + "\" }\n# secrets: " + secretBackend + "\n";
    }

    private static String lower(boolean value) {
        return value ? "true" : "false";
    }

    private static String formatNameList(List<String> names) {
        StringBuilder sb = new StringBuilder("[");
        for (int i = 0; i < names.size(); i++) {
            if (i > 0) {
                sb.append(", ");
            }
            sb.append("'").append(names.get(i)).append("'");
        }
        sb.append("]");
        return sb.toString();
    }
}

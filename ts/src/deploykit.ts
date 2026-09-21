/**
 * Deterministic, secure-by-default deployment plans for a reference LLM app
 * across the `local`, `azure`, and `aws` targets.
 *
 * This is a faithful port of the Python `deploykit` core: configuration
 * defaults, plan and manifest generation, target selection, and the
 * secure-by-default checks. It deliberately excludes the argparse CLI wiring
 * and any real cloud apply/destroy calls, which are stubs in Python as well.
 */

/** Deployment configuration, mirroring the Python `DeployConfig` dataclass. */
export interface Config {
  appName: string;
  image: string;
  replicas: number;
  tls: boolean;
  secretBackend: string;
  denyAllEgress: boolean;
}

/** Returns the same secure defaults as the Python `DeployConfig`. */
export function defaultConfig(): Config {
  return {
    appName: "ledgerrag",
    image: "ghcr.io/example/ledgerrag:latest",
    replicas: 2,
    tls: true,
    secretBackend: "auto",
    denyAllEgress: true,
  };
}

/** The rendered deployment plan for a single target. */
export interface Plan {
  target: string;
  steps: string[];
  manifests: Record<string, string>;
  warnings: string[];
}

/** Reports whether the plan tripped no secure-by-default warnings. */
export function isSecure(plan: Plan): boolean {
  return plan.warnings.length === 0;
}

/** The rendered plan text plus the intended process exit code. */
export interface RenderResult {
  text: string;
  code: number;
}

/** Renders a deployment {@link Plan} for a specific environment. */
export interface Target {
  readonly name: string;
  plan(cfg: Config): Plan;
}

/** The canonical target names, in registration order. */
export const TARGET_NAMES = ["local", "azure", "aws"] as const;

function securityChecks(cfg: Config): string[] {
  const warnings: string[] = [];
  if (!cfg.tls) {
    warnings.push("TLS disabled -- traffic would be unencrypted");
  }
  if (!cfg.denyAllEgress) {
    warnings.push("egress not restricted -- data exfil risk");
  }
  if (cfg.secretBackend === "plaintext") {
    warnings.push("secrets stored in plaintext");
  }
  return warnings;
}

function boolStr(value: boolean): string {
  return value ? "true" : "false";
}

function helmValues(cfg: Config, secretBackend: string): string {
  return (
    `image: ${cfg.image}\nreplicas: ${cfg.replicas}\n` +
    `tls: ${boolStr(cfg.tls)}\nsecretBackend: ${secretBackend}\n` +
    `networkPolicy:\n  denyAllEgress: ${boolStr(cfg.denyAllEgress)}\n`
  );
}

function bicep(cfg: Config, secretBackend: string): string {
  return (
    `// Bicep for ${cfg.appName}\nparam image string = '${cfg.image}'\n` +
    `param secretBackend string = '${secretBackend}'\n`
  );
}

function terraform(cfg: Config, secretBackend: string): string {
  return (
    `# Terraform for ${cfg.appName}\nvariable "image" { default = "${cfg.image}" }\n` +
    `# secrets: ${secretBackend}\n`
  );
}

const localTarget: Target = {
  name: "local",
  plan(cfg: Config): Plan {
    const secretBackend = cfg.secretBackend === "auto" ? "k8s-secret" : cfg.secretBackend;
    const steps = [
      "ensure kind/minikube cluster is running",
      `create namespace ${cfg.appName}`,
      `create ${secretBackend} for app credentials`,
      `helm install ${cfg.appName} ./charts/${cfg.appName} --set image=${cfg.image}`,
    ];
    if (cfg.denyAllEgress) {
      steps.push("apply default-deny NetworkPolicy");
    }
    return {
      target: "local",
      steps,
      manifests: { "values.yaml": helmValues(cfg, secretBackend) },
      warnings: securityChecks(cfg),
    };
  },
};

const azureTarget: Target = {
  name: "azure",
  plan(cfg: Config): Plan {
    const secretBackend = cfg.secretBackend === "auto" ? "keyvault" : cfg.secretBackend;
    const ingress = cfg.tls
      ? "bind managed identity; enable ingress with TLS"
      : "enable ingress";
    const steps = [
      "az group create + provision Azure Container Apps environment (Bicep)",
      `store secrets in Azure ${secretBackend}`,
      `deploy ${cfg.appName} container app (image=${cfg.image}, replicas=${cfg.replicas})`,
      ingress,
    ];
    if (cfg.denyAllEgress) {
      steps.push("apply egress restrictions via NSG / container app policy");
    }
    return {
      target: "azure",
      steps,
      manifests: { "main.bicep": bicep(cfg, secretBackend) },
      warnings: securityChecks(cfg),
    };
  },
};

const awsTarget: Target = {
  name: "aws",
  plan(cfg: Config): Plan {
    const secretBackend = cfg.secretBackend === "auto" ? "secrets-manager" : cfg.secretBackend;
    const front = cfg.tls ? "front with ALB + ACM TLS cert" : "front with ALB";
    const steps = [
      "terraform apply: VPC + ECS/Fargate cluster",
      `store secrets in AWS ${secretBackend}`,
      `deploy ${cfg.appName} service (image=${cfg.image}, desired_count=${cfg.replicas})`,
      front,
    ];
    if (cfg.denyAllEgress) {
      steps.push("apply restrictive security groups (deny-all egress baseline)");
    }
    return {
      target: "aws",
      steps,
      manifests: { "main.tf": terraform(cfg, secretBackend) },
      warnings: securityChecks(cfg),
    };
  },
};

const registry: Record<string, Target> = {
  local: localTarget,
  azure: azureTarget,
  aws: awsTarget,
};

/**
 * Resolves a target by name. Throws an `Error` whose message matches the Python
 * `ValueError` ("unknown target '<name>'. available: [...]").
 */
export function getTarget(name: string): Target {
  const target = registry[name];
  if (target === undefined) {
    const available = TARGET_NAMES.map((n) => `'${n}'`).join(", ");
    throw new Error(`unknown target '${name}'. available: [${available}]`);
  }
  return target;
}

/**
 * Renders the human-readable plan for `targetName` and returns the text with the
 * intended exit code (1 when secure-by-default warnings fire, otherwise 0).
 * Mirrors the Python CLI's `_print_plan`.
 */
export function renderPlan(targetName: string, cfg: Config): RenderResult {
  const plan = getTarget(targetName).plan(cfg);
  let out = `== DeployKit plan: ${plan.target} / ${cfg.appName} ==\n`;
  plan.steps.forEach((step, i) => {
    out += `  ${i + 1}. ${step}\n`;
  });
  if (plan.warnings.length > 0) {
    out += "\n  SECURITY WARNINGS:\n";
    for (const w of plan.warnings) {
      out += `    ! ${w}\n`;
    }
    return { text: out, code: 1 };
  }
  out += "\n  secure-by-default checks: PASS\n";
  return { text: out, code: 0 };
}

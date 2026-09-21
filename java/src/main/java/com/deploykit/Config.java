package com.deploykit;

/**
 * Deployment configuration, mirroring the Python {@code DeployConfig} dataclass.
 * The no-arg defaults are the secure-by-default choices.
 */
public final class Config {
    private String appName = "ledgerrag";
    private String image = "ghcr.io/example/ledgerrag:latest";
    private int replicas = 2;
    private boolean tls = true;
    private String secretBackend = "auto";
    private boolean denyAllEgress = true;

    public String getAppName() {
        return appName;
    }

    public void setAppName(String appName) {
        this.appName = appName;
    }

    public String getImage() {
        return image;
    }

    public void setImage(String image) {
        this.image = image;
    }

    public int getReplicas() {
        return replicas;
    }

    public void setReplicas(int replicas) {
        this.replicas = replicas;
    }

    public boolean isTls() {
        return tls;
    }

    public void setTls(boolean tls) {
        this.tls = tls;
    }

    public String getSecretBackend() {
        return secretBackend;
    }

    public void setSecretBackend(String secretBackend) {
        this.secretBackend = secretBackend;
    }

    public boolean isDenyAllEgress() {
        return denyAllEgress;
    }

    public void setDenyAllEgress(boolean denyAllEgress) {
        this.denyAllEgress = denyAllEgress;
    }
}

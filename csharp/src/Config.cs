namespace DeployKit;

/// <summary>
/// Deployment configuration, mirroring the Python <c>DeployConfig</c> dataclass.
/// The defaults are the secure-by-default choices.
/// </summary>
public sealed class Config
{
    public string AppName { get; set; } = "ledgerrag";
    public string Image { get; set; } = "ghcr.io/example/ledgerrag:latest";
    public int Replicas { get; set; } = 2;
    public bool Tls { get; set; } = true;
    public string SecretBackend { get; set; } = "auto";
    public bool DenyAllEgress { get; set; } = true;
}

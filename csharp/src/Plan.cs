using System.Collections.Generic;

namespace DeployKit;

/// <summary>The rendered deployment plan for a single target.</summary>
public sealed class Plan
{
    public string Target { get; }
    public IReadOnlyList<string> Steps { get; }
    public IReadOnlyDictionary<string, string> Manifests { get; }
    public IReadOnlyList<string> Warnings { get; }

    public Plan(
        string target,
        IReadOnlyList<string> steps,
        IReadOnlyDictionary<string, string> manifests,
        IReadOnlyList<string> warnings)
    {
        Target = target;
        Steps = steps;
        Manifests = manifests;
        Warnings = warnings;
    }

    /// <summary>Reports whether the plan tripped no secure-by-default warnings.</summary>
    public bool IsSecure() => Warnings.Count == 0;
}

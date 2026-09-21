package com.deploykit;

import java.util.List;
import java.util.Map;

/** The rendered deployment plan for a single target. */
public final class Plan {
    private final String target;
    private final List<String> steps;
    private final Map<String, String> manifests;
    private final List<String> warnings;

    public Plan(String target, List<String> steps, Map<String, String> manifests, List<String> warnings) {
        this.target = target;
        this.steps = steps;
        this.manifests = manifests;
        this.warnings = warnings;
    }

    public String getTarget() {
        return target;
    }

    public List<String> getSteps() {
        return steps;
    }

    public Map<String, String> getManifests() {
        return manifests;
    }

    public List<String> getWarnings() {
        return warnings;
    }

    /** Reports whether the plan tripped no secure-by-default warnings. */
    public boolean isSecure() {
        return warnings.isEmpty();
    }
}

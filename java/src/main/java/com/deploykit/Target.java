package com.deploykit;

/** Renders a deployment {@link Plan} for a specific environment. */
public interface Target {
    /** Returns the target's canonical name. */
    String name();

    /** Renders the deployment plan for {@code cfg}. */
    Plan plan(Config cfg);
}

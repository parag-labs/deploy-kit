package com.deploykit;

/** The rendered plan text plus the intended process exit code. */
public record RenderResult(String text, int code) {
}

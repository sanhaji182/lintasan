/**
 * Plugin system for Lintasan LLM proxy.
 * Provides lifecycle hooks: beforeRequest, afterRequest, onError, onStream.
 * Plugins are loaded from /plugins/ directory and executed in priority order.
 */

import { readdir } from "fs/promises";
import { pathToFileURL } from "url";
import path from "path";

const PLUGINS_DIR = path.resolve(process.cwd(), "plugins");

// In-memory plugin registry
let plugins = new Map();

/**
 * Load (or reload) all plugins from the plugins/ directory.
 * Hot-loadable: re-reads from disk each time it's called.
 */
export async function loadPlugins() {
  plugins.clear();
  let files;
  try {
    files = await readdir(PLUGINS_DIR);
  } catch (err) {
    if (err.code === "ENOENT") {
      console.warn("[plugins] plugins/ directory not found, skipping.");
      return;
    }
    throw err;
  }

  const jsFiles = files.filter((f) => f.endsWith(".js"));

  for (const file of jsFiles) {
    const filePath = path.join(PLUGINS_DIR, file);
    const fileUrl = pathToFileURL(filePath).href;
    try {
      // Cache-bust for hot reload
      const mod = await import(`${fileUrl}?t=${Date.now()}`);
      const plugin = mod.default || mod;
      if (plugin && plugin.name) {
        registerPlugin(plugin);
      } else {
        console.warn(`[plugins] ${file} does not export a valid plugin object (missing name).`);
      }
    } catch (err) {
      console.error(`[plugins] Failed to load ${file}:`, err.message);
    }
  }
}

/**
 * Register a plugin into the registry.
 */
export function registerPlugin(plugin) {
  const entry = {
    name: plugin.name,
    version: plugin.version || "1.0.0",
    enabled: plugin.enabled !== false,
    priority: plugin.priority ?? 100,
    hooks: {
      beforeRequest: plugin.hooks?.beforeRequest || null,
      afterRequest: plugin.hooks?.afterRequest || null,
      onError: plugin.hooks?.onError || null,
      onStream: plugin.hooks?.onStream || null,
    },
  };
  plugins.set(entry.name, entry);
}

/**
 * Unregister a plugin by name.
 */
export function unregisterPlugin(name) {
  return plugins.delete(name);
}

/**
 * List all registered plugins with their status.
 */
export function listPlugins() {
  return Array.from(plugins.values()).map((p) => ({
    name: p.name,
    version: p.version,
    enabled: p.enabled,
    priority: p.priority,
    hooks: Object.keys(p.hooks).filter((k) => p.hooks[k] !== null),
  }));
}

/**
 * Enable a plugin by name.
 */
export function enablePlugin(name) {
  const plugin = plugins.get(name);
  if (!plugin) return false;
  plugin.enabled = true;
  return true;
}

/**
 * Disable a plugin by name.
 */
export function disablePlugin(name) {
  const plugin = plugins.get(name);
  if (!plugin) return false;
  plugin.enabled = false;
  return true;
}

/**
 * Get enabled plugins sorted by priority (lower number = higher priority).
 */
function getEnabledSorted() {
  return Array.from(plugins.values())
    .filter((p) => p.enabled)
    .sort((a, b) => a.priority - b.priority);
}

/**
 * Run beforeRequest hooks on all enabled plugins in priority order.
 * If a plugin returns a response object, short-circuit and return it immediately.
 * Plugins can mutate ctx to transform the request.
 *
 * @param {object} ctx - { model, messages, stream, auth, headers, metadata }
 * @returns {object|null} - Short-circuit response or null to continue
 */
export async function runBeforeRequest(ctx) {
  const sorted = getEnabledSorted();
  for (const plugin of sorted) {
    if (plugin.hooks.beforeRequest) {
      try {
        const result = await plugin.hooks.beforeRequest(ctx);
        if (result && typeof result === "object" && result.__shortCircuit) {
          return result.response;
        }
      } catch (err) {
        console.error(`[plugins] ${plugin.name}.beforeRequest error:`, err.message);
      }
    }
  }
  return null;
}

/**
 * Run afterRequest hooks on all enabled plugins in priority order.
 * Plugins can transform the response.
 *
 * @param {object} ctx - Request context
 * @param {object} response - The response object
 * @returns {object} - Possibly transformed response
 */
export async function runAfterRequest(ctx, response) {
  const sorted = getEnabledSorted();
  let res = response;
  for (const plugin of sorted) {
    if (plugin.hooks.afterRequest) {
      try {
        const result = await plugin.hooks.afterRequest(ctx, res);
        if (result !== undefined && result !== null) {
          res = result;
        }
      } catch (err) {
        console.error(`[plugins] ${plugin.name}.afterRequest error:`, err.message);
      }
    }
  }
  return res;
}

/**
 * Run onError hooks on all enabled plugins in priority order.
 *
 * @param {object} ctx - Request context
 * @param {Error} error - The error
 * @returns {object|null} - Recovery response or null
 */
export async function runOnError(ctx, error) {
  const sorted = getEnabledSorted();
  for (const plugin of sorted) {
    if (plugin.hooks.onError) {
      try {
        const result = await plugin.hooks.onError(ctx, error);
        if (result && typeof result === "object" && result.__recovery) {
          return result.response;
        }
      } catch (err) {
        console.error(`[plugins] ${plugin.name}.onError error:`, err.message);
      }
    }
  }
  return null;
}

/**
 * Run onStream hooks on all enabled plugins for a stream chunk.
 *
 * @param {object} ctx - Request context
 * @param {string|object} chunk - Stream chunk
 * @returns {string|object} - Possibly transformed chunk
 */
export async function runOnStream(ctx, chunk) {
  const sorted = getEnabledSorted();
  let c = chunk;
  for (const plugin of sorted) {
    if (plugin.hooks.onStream) {
      try {
        const result = await plugin.hooks.onStream(ctx, c);
        if (result !== undefined && result !== null) {
          c = result;
        }
      } catch (err) {
        console.error(`[plugins] ${plugin.name}.onStream error:`, err.message);
      }
    }
  }
  return c;
}

/**
 * Helper to create a short-circuit response from a beforeRequest hook.
 */
export function shortCircuit(response) {
  return { __shortCircuit: true, response };
}

/**
 * Helper to create a recovery response from an onError hook.
 */
export function recover(response) {
  return { __recovery: true, response };
}

Only the JavaScript code.import fs from 'fs/promises';
import path from 'path';

export default {
  name: "log-every-request-to-a-json-file",
  version: "1.0.0",
  description: "Log every request to a JSON file at data/plugin-logs.json with timestamp, model, and token count",
  priority: 100,
  enabled: true,
  hooks: {
    afterRequest: async (ctx, response) => {
      const logPath = path.join(process.cwd(), 'data', 'plugin-logs.json');
      // Ensure the target directory exists
      await fs.mkdir(path.dirname(logPath), { recursive: true });

      let logs = [];
      try {
        const raw = await fs.readFile(logPath, 'utf-8');
        logs = JSON.parse(raw);
        if (!Array.isArray(logs)) logs = []; // safety in case file is corrupt
      } catch {
        // file does not exist or is invalid JSON, start fresh
        logs = [];
      }

      const tokenCount = response?.usage?.total_tokens ?? 0;
      const model = ctx.model || 'unknown';

      logs.push({
        timestamp: new Date().toISOString(),
        model,
        tokenCount
      });

      await fs.writeFile(logPath, JSON.stringify(logs, null, 2));
    }
  }
};
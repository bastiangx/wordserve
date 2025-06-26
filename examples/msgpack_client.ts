// TypeScript example client for simplified MessagePack IPC

const msgpack = require("@msgpack/msgpack");
const { spawn } = require("child_process");

// Interface matching Go server
interface CompletionRequest {
  p: string; // prefix
  l?: number; // limit
}

interface CompletionSuggestion {
  w: string; // word
  r: number; // rank
}

interface CompletionResponse {
  s: CompletionSuggestion[]; // suggestions
  c: number; // count
  t: number; // time taken (microseconds)
}

interface CompletionError {
  e: string; // error
  c: number; // code
}

async function testCompletion(prefix: string, limit: number = 10): Promise<void> {
  return new Promise((resolve, reject) => {
    const typerExec = spawn("./typer", ["server"], { stdio: ["pipe", "pipe", "pipe"] });
    
    const request: CompletionRequest = { p: prefix };
    if (limit && limit > 0) {
      request.l = limit;
    }

    console.log(`Sending request: prefix="${prefix}", limit=${limit}`);
    
    // Encode and send request
    const encodedRequest = msgpack.encode(request);
    typerExec.stdin.write(encodedRequest);
    typerExec.stdin.end();

    let responseData = Buffer.alloc(0);
    
    typerExec.stdout.on("data", (chunk: Buffer) => {
      responseData = Buffer.concat([responseData, chunk]);
    });

    typerExec.stdout.on("end", () => {
      try {
        const response = msgpack.decode(responseData) as CompletionResponse | CompletionError;
        
        if ('e' in response) {
          // Error response
          console.error(`Error: ${response.e} (code: ${response.c})`);
          reject(new Error(response.e));
        } else {
          // Success response
          console.log(`\nCompletion Results for "${prefix}":`);
          console.log(`Count: ${response.c}`);
          console.log(`Time: ${response.t} microseconds`);
          console.log("Suggestions:");
          
          response.s.forEach((suggestion, index) => {
            console.log(`  ${index + 1}. ${suggestion.w} (rank: ${suggestion.r}, freq: ${suggestion.f || 'N/A'})`);
          });
          
          resolve();
        }
      } catch (error) {
        console.error("Failed to decode response:", error);
        reject(error);
      }
    });

    typerExec.stderr.on("data", (data) => {
      console.error(`stderr: ${data}`);
    });

    typerExec.on("close", (code) => {
      if (code !== 0) {
        console.error(`typer process exited with code ${code}`);
        reject(new Error(`Process failed with code ${code}`));
      }
    });

    typerExec.on("error", (error) => {
      console.error(`Failed to start typer process:", error);
      reject(error);
    });
  });
}

// Test multiple prefixes
async function runTests() {
  const testCases = [
    { prefix: "hello", limit: 5 },
    { prefix: "test", limit: 3 },
    { prefix: "com", limit: 10 },
  ];

  for (const testCase of testCases) {
    try {
      await testCompletion(testCase.prefix, testCase.limit);
      console.log("\n" + "-".repeat(50) + "\n");
    } catch (error) {
      console.error(`Test failed for prefix "${testCase.prefix}":`, error);
    }
  }
}

// Run tests
runTests().catch(console.error);


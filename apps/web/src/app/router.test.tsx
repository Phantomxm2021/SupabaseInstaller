import { QueryClient } from "@tanstack/react-query";
import { isValidElement } from "react";
import { matchRoutes, Navigate } from "react-router-dom";
import { ConfigurationPage } from "../features/project/configuration/ConfigurationPage";
import { FunctionsPage } from "../features/project/FunctionsPage";
import { FunctionSecretsPage } from "../features/project/FunctionSecretsPage";
import { FunctionLogsPage } from "../features/project/FunctionLogsPage";
import {
  EmailsRoute,
  RetainedAuthenticationRedirect,
  SignInProvidersRoute,
  URLConfigurationRoute,
} from "../features/authentication/AuthenticationWorkspace";
import { createAppRouter, NotFoundPage } from "./router";

it("does not register removed project configuration compatibility routes", () => {
  const router = createAppRouter(new QueryClient());
  const shellRoute = router.routes.find((route) =>
    route.children?.some((child) => child.path === "/projects/:projectId"),
  );
  const projectRoute = shellRoute?.children?.find(
    (route) => route.path === "/projects/:projectId",
  );
  const childPaths = projectRoute?.children?.map((route) => route.path) ?? [];
  expect(childPaths).toContain("configuration");
  expect(childPaths).toContain("authentication");
  expect(childPaths).toContain("functions");
  expect(childPaths).not.toEqual(
    expect.arrayContaining([
      "services",
      "database",
      "storage",
      "realtime",
      "pooler",
      "network",
      "secrets",
      "settings",
    ]),
  );
  expect(childPaths).toContain("*");
  const elementType = (element: unknown) =>
    isValidElement(element) ? element.type : undefined;
  expect(
    elementType(
      projectRoute?.children?.find((route) => route.path === "*")?.element,
    ),
  ).toBe(NotFoundPage);
  expect(
    elementType(
      matchRoutes(router.routes, "/projects/bee/configuration")?.at(-1)?.route
        .element,
    ),
  ).toBe(ConfigurationPage);
  expect(
    elementType(matchRoutes(router.routes, "/projects/bee/functions")?.at(-1)?.route.element),
  ).toBe(FunctionsPage);
  const functionsRoute = projectRoute?.children?.find(
    (route) => route.path === "functions",
  );
  expect(functionsRoute?.children?.map((route) => route.path)).toContain("secrets");
  expect(functionsRoute?.children?.map((route) => route.path)).toEqual([undefined, "secrets", ":functionName/logs"]);
  expect(elementType(matchRoutes(router.routes, "/projects/bee/functions/demo/logs")?.at(-1)?.route.element)).toBe(FunctionLogsPage);
  expect(
    elementType(
      matchRoutes(router.routes, "/projects/bee/functions/secrets")?.at(-1)
        ?.route.element,
    ),
  ).toBe(FunctionSecretsPage);
  const authenticationRoute = projectRoute?.children?.find(
    (route) => route.path === "authentication",
  );
  const authenticationPaths =
    authenticationRoute?.children?.map((route) => route.path) ?? [];
  expect(authenticationPaths).toEqual(
    expect.arrayContaining([
      "sign-in-providers",
      "emails",
      "url-configuration",
    ]),
  );
  const authenticationIndex = authenticationRoute?.children?.find(
    (route) => route.index,
  );
  expect(elementType(authenticationIndex?.element)).toBe(Navigate);
  expect(
    isValidElement<{ to: string }>(authenticationIndex?.element) &&
      authenticationIndex.element.props.to,
  ).toBe("emails");
  expect(
    elementType(
      matchRoutes(router.routes, "/projects/bee/authentication/users")?.at(-1)
        ?.route.element,
    ),
  ).toBe(RetainedAuthenticationRedirect);
  expect(
    elementType(
      matchRoutes(router.routes, "/projects/bee/authentication/oauth-apps")?.at(
        -1,
      )?.route.element,
    ),
  ).toBe(RetainedAuthenticationRedirect);
  expect(
    elementType(
      matchRoutes(
        router.routes,
        "/projects/bee/authentication/sign-in-providers",
      )?.at(-1)?.route.element,
    ),
  ).toBe(SignInProvidersRoute);
  expect(
    elementType(
      matchRoutes(router.routes, "/projects/bee/authentication/emails")?.at(-1)
        ?.route.element,
    ),
  ).toBe(EmailsRoute);
  expect(
    elementType(
      matchRoutes(
        router.routes,
        "/projects/bee/authentication/url-configuration",
      )?.at(-1)?.route.element,
    ),
  ).toBe(URLConfigurationRoute);
  for (const path of [
    "services",
    "database",
    "storage",
    "realtime",
    "pooler",
    "network",
    "secrets",
    "settings",
  ]) {
    expect(
      elementType(
        matchRoutes(router.routes, `/projects/bee/${path}`)?.at(-1)?.route
          .element,
      ),
    ).toBe(NotFoundPage);
  }
  router.dispose();
});

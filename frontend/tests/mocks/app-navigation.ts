export interface NavigationLike {
  cancel: () => void;
  willUnload?: boolean;
}

export function beforeNavigate(_handler: (navigation: NavigationLike) => void) {}
